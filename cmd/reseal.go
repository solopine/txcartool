package main

import (
	"bufio"
	"bytes"
	"context"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"github.com/solopine/txcartool/lib/filestore"
	"github.com/solopine/txcartool/lib/harmonydb"
	"github.com/solopine/txcartool/lib/shared"
	"github.com/solopine/txcartool/util"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Reseal(cctx *cli.Context) error {
	nodeApi, closer, err := lcli.GetFullNodeAPIV1(cctx)
	if err != nil {
		return err
	}
	defer closer()

	maddr, err := util.GetActorAddress(cctx)
	if err != nil {
		return err
	}

	sectorSize, nv, err := util.GetSectorSize(context.Background(), nodeApi, maddr)
	if err != nil {
		return err
	}

	sdir := cctx.String("seal-dir")
	if sdir == "" {
		home, _ := os.LookupEnv("HOME")
		if home == "" {
			return xerrors.New("No storage directory is set and get $HOME fail.")
		}
		sdir = filepath.Join(home, "redo")
		log.Infow("No storage directory is set, the default directory will be used", "path", sdir)
	}

	storageDir := cctx.String("storage-dir")
	for _, path := range []string{sdir, storageDir} {
		if path == "" {
			continue
		}

		for _, t := range storiface.PathTypes {
			p := filepath.Join(path, t.String())
			if _, err := os.Stat(p); err != nil {
				if err := os.MkdirAll(filepath.Join(path, t.String()), 0755); err != nil {
					return err
				}
			}
		}
	}

	ctx := cctx.Context
	//db
	hosts := []string{cctx.String("dbhost")}
	username := cctx.String("dbuser")
	password := cctx.String("dbpwd")
	database := "yugabyte"
	port := "5433"
	itestID := harmonydb.ITestID("")
	db, err := harmonydb.New(hosts, username, password, database, port, itestID)
	if err != nil {
		return err
	}

	sbfs := &basicfs.Provider{
		Root: sdir,
	}

	sb, err := ffiwrapper.New(sbfs)
	if err != nil {
		return err
	}

	amid, err := addr.IDFromAddress(maddr)
	if err != nil {
		return err
	}
	actor := abi.ActorID(amid)

	spt, err := miner.SealProofTypeFromSectorSize(sectorSize, nv, miner.SealProofVariant_Standard)
	if err != nil {
		return err
	}

	sidFilePath := cctx.String("sid-file")
	sectorSealInfos, err := ReadSectorSealInfos(sidFilePath)
	if err != nil {
		return err
	}

	smax := cctx.Int("smax")
	p1max := cctx.Int("p1max")

	sectorsThrottle := make(chan struct{}, smax)

	apThrottle := make(chan struct{}, 1)
	p1Throttle := make(chan struct{}, p1max)
	p2Throttle := make(chan struct{}, 1)
	finThrottle := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(len(sectorSealInfos))

	// do the job
	for _, sectorSealInfo := range sectorSealInfos {

		go func(sectorSealInfo SectorSealInfo) {
			defer func() {
				<-sectorsThrottle
				wg.Done()
			}()
			sectorsThrottle <- struct{}{}

			sid := sectorSealInfo.sid
			log.Infof("try redo sector: %d. sectorsThrottle: %d", sid, len(sectorsThrottle))

			var sectorMetas []SectorMeta

			err = db.Select(ctx, &sectorMetas, `select sp_id, sector_num,ticket_epoch, ticket_value, orig_unsealed_cid, orig_sealed_cid, msg_cid_precommit from sectors_meta where sector_num=$1`, uint64(sid))
			if err != nil {
				log.Errorf("db:%+v", err)
				return
			}
			if len(sectorMetas) != 1 {
				log.Errorw("got from db", "sectorMetas", len(sectorMetas))

				err = db.Select(ctx, &sectorMetas, `select sp_id, sector_number as sector_num,ticket_epoch, ticket_value, tree_d_cid as orig_unsealed_cid, tree_r_cid as orig_sealed_cid, precommit_msg_cid as msg_cid_precommit from sectors_sdr_pipeline where sector_number=$1`, uint64(sid))
				if err != nil {
					log.Errorf("db2:%+v", err)
					return
				}
				if len(sectorMetas) != 1 {
					log.Errorw("got from db2", "sectorMetas", len(sectorMetas))
					return
				}
			}
			sectorMeta := sectorMetas[0]
			log.Infow("got from db", "sectorMeta", sectorMeta)

			//AP
			apResult, err := func() (APResult, error) {
				defer func() {
					<-apThrottle
				}()

				apThrottle <- struct{}{}

				log.Warnf("start to process AP sector: %d. apThrottle: %d", sid, len(apThrottle))

				pi, err := addPiece(sectorSealInfo, actor, sectorSize, spt, sb, &sectorMeta)
				if err != nil {
					log.Errorf("AP seal error for %d, err: %s", sid, err)
					return APResult{}, err
				}
				log.Warnw("end to process AP", "sector", sid, "pieceInfo", pi)
				return APResult{sid, actor, spt, sb, pi}, nil

			}()
			if err != nil {
				log.Error(err)
				return
			}

			if sectorSealInfo.sealType == "AP" {
				return
			}

			// p1
			p1result, err := func() (P1Result, error) {
				defer func() {
					<-p1Throttle
				}()
				p1Throttle <- struct{}{}

				log.Warnf("start to process P1 sector: %d. p1Throttle: %d", sid, len(p1Throttle))

				time.Sleep(5 * time.Second)

				p1Out, err := precommit1(apResult, sid, actor, spt, sb, &sectorMeta)
				if err != nil {
					log.Errorf("P1 seal error for %d, err: %s", sid, err)
					return P1Result{}, err
				}

				log.Warnf("end to process P1 sector: %d", sid)
				return P1Result{sid, actor, spt, sb, p1Out}, nil

			}()

			if err != nil {
				log.Error(err)
				return
			}

			if sectorSealInfo.sealType == "P1" {
				return
			}

			// P2
			p2result, err := func() (P2Result, error) {
				defer func() {
					<-p2Throttle
				}()
				p2Throttle <- struct{}{}

				log.Warnf("start to process P2 sector: %d. p2Throttle: %d", sid, len(p2Throttle))

				err := precommit2(p1result.sid, p1result.actor, p1result.spt, p1result.sb, p1result.p1Out, &sectorMeta)
				if err != nil {
					log.Errorf("P2 seal error for %d, err: %s", p1result.sid, err)
					return P2Result{}, err
				}
				return P2Result{p1result.sid, actor, spt, sb, sdir, storageDir}, nil
			}()
			if err != nil {
				log.Error(err)
				return
			}

			if sectorSealInfo.sealType == "P2" {
				return
			}

			// fin
			func() {
				defer func() {
					<-finThrottle
				}()
				finThrottle <- struct{}{}

				log.Warnf("start to process FIN sector: %d. finThrottle: %d", sid, len(finThrottle))

				err := finalize(p2result.sid, p2result.actor, p2result.spt, p2result.sb, p2result.sdir, p2result.storageDir)
				if err != nil {
					log.Errorf("fin seal error for %d, err: %s", p2result.sid, err)
					return
				}
				log.Warnf("end to process fin sector: %d", p2result.sid)
			}()

			log.Warnf("end to process sector: %d", sid)

		}(sectorSealInfo)
	}

	wg.Wait()
	log.Warnf("end redo")
	return nil
}

func addPiece(sectorSealInfo SectorSealInfo, actor abi.ActorID,
	sectorSize abi.SectorSize, spt abi.RegisteredSealProof,
	sb *ffiwrapper.Sealer,
	sectorMeta *SectorMeta,
) (abi.PieceInfo, error) {
	dbOrigUnsealedCid, err := cid.Decode(sectorMeta.OrigUnsealedCid)
	if err != nil {
		log.Errorw("cid.Decode error", "err", err)
		return abi.PieceInfo{}, err
	}

	sid := sectorSealInfo.sid
	sidRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  actor,
			Number: sid,
		},
		ProofType: spt,
	}

	pieceSize := abi.PaddedPieceSize(sectorSize).Unpadded()

	// for DC
	log.Infow("add piece for DC", "sid", sid, "carKey", sectorSealInfo.carKey)

	carKey := sectorSealInfo.carKey
	r, err := genDCAndReturnReader(context.TODO(), sidRef, pieceSize, carKey)
	if err != nil {
		log.Errorw("genDCAndReturnReader error", "err", err)
		return abi.PieceInfo{}, err
	}
	pi, err := sb.AddPiece(context.TODO(), sidRef, nil, abi.PaddedPieceSize(sectorSize).Unpadded(), r)
	if err != nil {
		log.Errorw("AddPiece error", "err", err)
		return abi.PieceInfo{}, err
	}
	log.Infow("AddPiece", "pi", pi)

	if pi.PieceCID.String() != dbOrigUnsealedCid.String() {
		log.Errorw("pi.PieceCID != dbOrigUnsealedCid", "pi.PieceCID", pi.PieceCID.String(), "dbOrigUnsealedCid", dbOrigUnsealedCid.String())
		return abi.PieceInfo{}, err
	}

	return pi, nil
}

func precommit1(apResult APResult,
	sid abi.SectorNumber,
	actor abi.ActorID,
	spt abi.RegisteredSealProof,
	sb *ffiwrapper.Sealer,
	sectorMeta *SectorMeta,
) (storiface.PreCommit1Out, error) {

	sidRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  actor,
			Number: sid,
		},
		ProofType: spt,
	}

	log.Infow("si.Pieces", "apResult.pi", apResult.pi)

	//2. p1
	p1Out, err := sb.SealPreCommit1(context.TODO(), sidRef, sectorMeta.TicketValue, []abi.PieceInfo{apResult.pi})
	if err != nil {
		log.Errorw("SealPreCommit1 error", "err", err)
		return nil, err
	}
	log.Infow("SealPreCommit1", "p1Out", p1Out)

	return p1Out, nil
}

func precommit2(sid abi.SectorNumber, actor abi.ActorID,
	spt abi.RegisteredSealProof, sb *ffiwrapper.Sealer,
	p1Out storiface.PreCommit1Out,
	sectorMeta *SectorMeta) error {

	dbOrigSealedCid, err := cid.Decode(sectorMeta.OrigSealedCid)
	if err != nil {
		log.Errorw("cid.Decode error", "err", err)
		return err
	}

	sidRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  actor,
			Number: sid,
		},
		ProofType: spt,
	}

	//3. p2
	cids, err := sb.SealPreCommit2(context.TODO(), sidRef, p1Out)
	if err != nil {
		log.Errorw("SealPreCommit2 error", "err", err)
		return err
	}
	if cids.Sealed.String() != dbOrigSealedCid.String() {
		log.Errorw("SealPreCommit2 result is invalid, different from that on the chain", "result-cod", cids.Sealed.String(), "chain-cid", dbOrigSealedCid.String())
		return xerrors.Errorf("cids.Sealed.String() != commR.String()")
	}
	log.Infow("SealPreCommit2", "cids", cids)

	return nil
}

func finalize(sid abi.SectorNumber, actor abi.ActorID,

	spt abi.RegisteredSealProof, sb *ffiwrapper.Sealer,
	sdir string,
	storageDir string) error {

	sidRef := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  actor,
			Number: sid,
		},
		ProofType: spt,
	}

	//4. fin
	err := sb.FinalizeSector(context.TODO(), sidRef)
	if err != nil {
		log.Errorw("FinalizeSector error", "err", err)
		return err
	}
	log.Infow("FinalizeSector", "sid", sid)

	//5

	for _, pt := range storiface.PathTypes {
		if pt != storiface.FTSealed && pt != storiface.FTCache {
			continue // Currently only CC sector is supported
		}

		err := move(filepath.Join(sdir, pt.String(), storiface.SectorName(sidRef.ID)), filepath.Join(storageDir, pt.String(), storiface.SectorName(sidRef.ID)))
		if err != nil {
			log.Errorw("move sector fail", "err", err, "sid", sid)
			return err
		}
		log.Infow("move sector successful", "sid", sid, "pt", pt)
	}

	unsealedFilePath := filepath.Join(sdir, storiface.FTUnsealed.String(), storiface.SectorName(sidRef.ID))
	err = removeFile(unsealedFilePath)
	if err != nil {
		log.Errorw("rm sector fail", "err", err, "sid", sid)
		return err
	}

	log.Infow("FinalizeSector successful", "sid", sid)

	return nil
}

func move(from, to string) error {
	from, err := homedir.Expand(from)
	if err != nil {
		return xerrors.Errorf("move: expanding from: %w", err)
	}

	to, err = homedir.Expand(to)
	if err != nil {
		return xerrors.Errorf("move: expanding to: %w", err)
	}

	if filepath.Base(from) != filepath.Base(to) {
		return xerrors.Errorf("move: base names must match ('%s' != '%s')", filepath.Base(from), filepath.Base(to))
	}

	log.Debugw("move sector data", "from", from, "to", to)

	toDir := filepath.Dir(to)

	// `mv` has decades of experience in moving files quickly; don't pretend we
	//  can do better

	var errOut bytes.Buffer

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		if err := os.MkdirAll(toDir, 0777); err != nil {
			return xerrors.Errorf("failed exec MkdirAll: %s", err)
		}

		cmd = exec.Command("/usr/bin/env", "mv", from, toDir) // nolint
	} else {
		cmd = exec.Command("/usr/bin/env", "mv", "-t", toDir, from) // nolint
	}

	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("exec mv (stderr: %s): %w", strings.TrimSpace(errOut.String()), err)
	}

	return nil
}

func removeFile(filePath string) error {
	filePath, err := homedir.Expand(filePath)
	if err != nil {
		return xerrors.Errorf("removeFile: expanding from: %w", err)
	}

	log.Debugw("removeFile sector data", "filePath", filePath)

	var errOut bytes.Buffer

	var cmd *exec.Cmd
	cmd = exec.Command("/usr/bin/env", "rm", filePath) // nolint

	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("exec rm (stderr: %s): %w", strings.TrimSpace(errOut.String()), err)
	}

	return nil
}

func ReadSectorSealInfos(path string) ([]SectorSealInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var sectorSealInfos []SectorSealInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		log.Infof("process line:%s", line)

		if strings.TrimSpace(line) == "" {
			continue
		}

		lineParts := strings.Split(line, " ")
		log.Infow("line Split", "lineParts", lineParts)

		var sectorSealInfo SectorSealInfo
		// sid
		sidStr := lineParts[0]
		sid, err := strconv.Atoi(sidStr)
		if err != nil {
			log.Errorw("sid parse fail", "err", err)
			return nil, err
		}
		sectorSealInfo.sid = abi.SectorNumber(sid)

		// sealType
		sectorSealInfo.sealType = lineParts[1]

		// isDC
		dcStr := lineParts[2]
		dcParts := strings.Split(dcStr, ":")
		sectorSealInfo.isDC = dcParts[0] == "DC"

		// carKey
		if sectorSealInfo.isDC {
			carKeyStr := dcParts[1]
			carKey, err := uuid.Parse(carKeyStr)
			if err != nil {
				log.Errorw("carKey parse fail", "err", err)
				return nil, err
			}
			sectorSealInfo.carKey = carKey
		}

		sectorSealInfos = append(sectorSealInfos, sectorSealInfo)
	}
	return sectorSealInfos, scanner.Err()
}

type APResult struct {
	sid   abi.SectorNumber
	actor abi.ActorID
	spt   abi.RegisteredSealProof
	sb    *ffiwrapper.Sealer
	pi    abi.PieceInfo
}

type P1Result struct {
	sid   abi.SectorNumber
	actor abi.ActorID
	spt   abi.RegisteredSealProof
	sb    *ffiwrapper.Sealer
	p1Out storiface.PreCommit1Out
}

type P2Result struct {
	sid        abi.SectorNumber
	actor      abi.ActorID
	spt        abi.RegisteredSealProof
	sb         *ffiwrapper.Sealer
	sdir       string
	storageDir string
}

type SectorSealInfo struct {
	sid      abi.SectorNumber
	sealType string
	isDC     bool
	carKey   uuid.UUID
}

func genDCAndReturnReader(ctx context.Context, sector storiface.SectorRef, pieceSize abi.UnpaddedPieceSize, carKey uuid.UUID) (storiface.Data, error) {

	carFile, err := internalCreateCar(ctx, carKey)
	if err != nil {
		return nil, err
	}

	// maker reader
	dcfs, err := filestore.NewLocalFileStore("/")
	if err != nil {
		return nil, err
	}

	dcFile, err := dcfs.Open(filestore.Path(carFile))
	if err != nil {
		log.Errorw("genDCAndReturnReader.dcfs.Open", "sector", sector)
		return nil, err
	}

	paddedReader, err := shared.NewInflatorReader(dcFile, uint64(dcFile.Size()), pieceSize)
	if err != nil {
		log.Errorw("genDCAndReturnReader.NewInflatorReader", "sector", sector)
		return nil, err
	}
	return paddedReader, nil
}

type SectorMeta struct {
	SpID            int64  `db:"sp_id"`
	SectorNum       int64  `db:"sector_num"`
	TicketEpoch     int64  `db:"ticket_epoch"`
	TicketValue     []byte `db:"ticket_value"`
	OrigUnsealedCid string `db:"orig_unsealed_cid"`
	OrigSealedCid   string `db:"orig_sealed_cid"`
	MsgCidPrecommit string `db:"msg_cid_precommit"`
	Seed            []byte `db:"seed_value"`
}
