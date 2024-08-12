package lib

import (
	"fmt"
	"github.com/filecoin-project/go-commp-utils/writer"
	"github.com/ipfs/go-cid"
	"io"
	"os"
)

func GenCommp(carPath string) (cid.Cid, int64, int64, error) {

	var pieceCid cid.Cid
	rdr, err := os.Open(carPath)
	if err != nil {
		return pieceCid, 0, 0, nil
	}
	defer rdr.Close() //nolint:errcheck

	w := &writer.Writer{}
	_, err = io.CopyBuffer(w, rdr, make([]byte, writer.CommPBuf))
	if err != nil {
		return pieceCid, 0, 0, fmt.Errorf("copy into commp writer: %w", err)
	}

	commp, err := w.Sum()
	if err != nil {
		return pieceCid, 0, 0, fmt.Errorf("computing commP failed: %w", err)
	}

	stat, err := os.Stat(carPath)
	if err != nil {
		return pieceCid, 0, 0, err
	}

	pieceCid = commp.PieceCID
	pieceSize := int64(commp.PieceSize)
	carSize := stat.Size()

	return pieceCid, pieceSize, carSize, err
}
