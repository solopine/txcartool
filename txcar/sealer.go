package txcar

import (
	"bytes"
	"context"
	"fmt"
	ffi "github.com/filecoin-project/filecoin-ffi"
	commpffi "github.com/filecoin-project/go-commp-utils/ffiwrapper"
	"github.com/filecoin-project/go-commp-utils/writer"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/partialfile"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
	"io"
	"os"
	"runtime"
)

func GenCommp(carPath string) (writer.DataCIDSize, error) {

	rdr, err := os.Open(carPath)
	if err != nil {
		return writer.DataCIDSize{}, nil
	}
	defer rdr.Close() //nolint:errcheck

	w := &writer.Writer{}
	_, err = io.CopyBuffer(w, rdr, make([]byte, writer.CommPBuf))
	if err != nil {
		return writer.DataCIDSize{}, fmt.Errorf("copy into commp writer: %w", err)
	}

	return w.Sum()
}

func AddPiece(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data, carFile string) (string, abi.PieceInfo, error) {
	origPieceData := pieceData
	defer func() {
		closer, ok := origPieceData.(io.Closer)
		if !ok {
			log.Debugf("AddPiece: cannot close pieceData reader %T because it is not an io.Closer", origPieceData)
			return
		}
		if err := closer.Close(); err != nil {
			log.Warnw("closing pieceData in AddPiece", "error", err)
		}
	}()

	// TODO: allow tuning those:
	chunk := abi.PaddedPieceSize(4 << 20)
	parallel := runtime.NumCPU()

	var offset abi.UnpaddedPieceSize

	maxPieceSize := SupportedSectorSize
	if pieceSize.Padded() != maxPieceSize {
		return "", abi.PieceInfo{}, xerrors.Errorf("pieceSize.Padded( %w ) != maxPieceSize(ssize:%w)", pieceSize.Padded(), maxPieceSize)
	}

	var done func()
	var stagedFile *partialfile.PartialFile

	defer func() {
		if done != nil {
			done()
		}

		if stagedFile != nil {
			if err := stagedFile.Close(); err != nil {
				log.Errorf("closing staged file: %+v", err)
			}
		}
	}()

	unsealedFilePath := carFile + ".unsealed"

	stagedFile, err := partialfile.CreatePartialFile(maxPieceSize, unsealedFilePath)
	if err != nil {
		return "", abi.PieceInfo{}, xerrors.Errorf("creating unsealed sector file: %w", err)
	}

	w, err := stagedFile.Writer(storiface.UnpaddedByteIndex(offset).Padded(), pieceSize.Padded())
	if err != nil {
		return "", abi.PieceInfo{}, xerrors.Errorf("getting partial file writer: %w", err)
	}

	pw := fr32.NewPadWriter(w)

	pr := io.TeeReader(io.LimitReader(pieceData, int64(pieceSize)), pw)

	throttle := make(chan []byte, parallel)
	piecePromises := make([]func() (abi.PieceInfo, error), 0)

	buf := make([]byte, chunk.Unpadded())
	for i := 0; i < parallel; i++ {
		if abi.UnpaddedPieceSize(i)*chunk.Unpadded() >= pieceSize {
			break // won't use this many buffers
		}
		throttle <- make([]byte, chunk.Unpadded())
	}

	for {
		var read int
		for rbuf := buf; len(rbuf) > 0; {
			n, err := pr.Read(rbuf)
			if err != nil && err != io.EOF {
				return "", abi.PieceInfo{}, xerrors.Errorf("pr read error: %w", err)
			}

			rbuf = rbuf[n:]
			read += n

			if err == io.EOF {
				break
			}
		}
		if read == 0 {
			break
		}

		done := make(chan struct {
			cid.Cid
			error
		}, 1)
		pbuf := <-throttle
		copy(pbuf, buf[:read])

		go func(read int) {
			defer func() {
				throttle <- pbuf
			}()

			c, err := pieceCid(SupportedRegisteredSealProof, pbuf[:read])
			done <- struct {
				cid.Cid
				error
			}{c, err}
		}(read)

		piecePromises = append(piecePromises, func() (abi.PieceInfo, error) {
			select {
			case e := <-done:
				if e.error != nil {
					return abi.PieceInfo{}, e.error
				}

				return abi.PieceInfo{
					Size:     abi.UnpaddedPieceSize(len(buf[:read])).Padded(),
					PieceCID: e.Cid,
				}, nil
			case <-ctx.Done():
				return abi.PieceInfo{}, ctx.Err()
			}
		})
	}

	if err := pw.Close(); err != nil {
		return "", abi.PieceInfo{}, xerrors.Errorf("closing padded writer: %w", err)
	}

	if err := stagedFile.MarkAllocated(storiface.UnpaddedByteIndex(offset).Padded(), pieceSize.Padded()); err != nil {
		return "", abi.PieceInfo{}, xerrors.Errorf("marking data range as allocated: %w", err)
	}

	if err := stagedFile.Close(); err != nil {
		return "", abi.PieceInfo{}, err
	}
	stagedFile = nil

	/*  #nosec G601 -- length is verified */
	if len(piecePromises) == 1 {
		p := piecePromises[0]
		pieceInfo, err := p()
		if err != nil {
			return "", abi.PieceInfo{}, err
		}
		return "", pieceInfo, nil
	}

	var payloadRoundedBytes abi.PaddedPieceSize
	pieceCids := make([]abi.PieceInfo, len(piecePromises))
	for i, promise := range piecePromises {
		pinfo, err := promise()
		if err != nil {
			return "", abi.PieceInfo{}, err
		}

		pieceCids[i] = pinfo
		payloadRoundedBytes += pinfo.Size
	}

	pieceCID, err := ffi.GenerateUnsealedCID(SupportedRegisteredSealProof, pieceCids)
	if err != nil {
		return "", abi.PieceInfo{}, xerrors.Errorf("generate unsealed CID: %w", err)
	}

	// validate that the pieceCID was properly formed
	if _, err := commcid.CIDToPieceCommitmentV1(pieceCID); err != nil {
		return "", abi.PieceInfo{}, err
	}

	if payloadRoundedBytes < pieceSize.Padded() {
		paddedCid, err := commpffi.ZeroPadPieceCommitment(pieceCID, payloadRoundedBytes.Unpadded(), pieceSize)
		if err != nil {
			return "", abi.PieceInfo{}, xerrors.Errorf("failed to pad data: %w", err)
		}

		pieceCID = paddedCid
	}

	if err := os.Truncate(unsealedFilePath, int64(pieceSize.Padded())); err != nil {
		return "", abi.PieceInfo{}, err
	}

	return unsealedFilePath, abi.PieceInfo{
		Size:     pieceSize.Padded(),
		PieceCID: pieceCID,
	}, nil
}

func pieceCid(spt abi.RegisteredSealProof, in []byte) (cid.Cid, error) {
	prf, werr, err := commpffi.ToReadableFile(bytes.NewReader(in), int64(len(in)))
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting tee reader pipe: %w", err)
	}

	pieceCID, err := ffi.GeneratePieceCIDFromFile(spt, prf, abi.UnpaddedPieceSize(len(in)))
	if err != nil {
		return cid.Undef, xerrors.Errorf("generating piece commitment: %w", err)
	}

	_ = prf.Close()

	return pieceCID, werr()
}
