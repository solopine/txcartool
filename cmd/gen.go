package main

import (
	"bytes"
	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper/basicfs"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
	"github.com/solopine/txcartool/lib/harmonydb"
	"github.com/solopine/txcartool/lib/util"
	"github.com/urfave/cli/v2"
)

func GenC1(cctx *cli.Context) error {
	log.Info("GenC1 begin")
	ctx := cctx.Context
	nodeApi, closer, err := lcli.GetFullNodeAPIV1(cctx)
	if err != nil {
		return err
	}
	defer closer()

	maddr, err := util.GetActorAddress(cctx)
	if err != nil {
		return err
	}

	sectorSize, nv, err := util.GetSectorSize(ctx, nodeApi, maddr)
	if err != nil {
		return err
	}

	sdir := cctx.String("seal-dir")
	sbfs := &basicfs.Provider{
		Root: sdir,
	}
	sb, err := ffiwrapper.New(sbfs)
	if err != nil {
		return err
	}

	//
	sid := cctx.Int("sid")

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

	amid, err := addr.IDFromAddress(maddr)
	if err != nil {
		return err
	}
	actor := abi.ActorID(amid)

	spt, err := miner.SealProofTypeFromSectorSize(sectorSize, nv, miner.SealProofVariant_Standard)
	if err != nil {
		return err
	}

	sector := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  actor,
			Number: abi.SectorNumber(sid),
		},
		ProofType: spt,
	}

	//
	var sectorMetas []SectorMeta
	err = db.Select(ctx, &sectorMetas, `select sp_id, sector_num,ticket_epoch, ticket_value, orig_unsealed_cid, orig_sealed_cid, msg_cid_precommit from sectors_meta where sector_num=$1`, uint64(sid))
	if err != nil {
		log.Errorf("db:%+v", err)
		return err
	}
	if len(sectorMetas) != 1 {
		log.Errorw("got from db", "sectorMetas", len(sectorMetas))

		err = db.Select(ctx, &sectorMetas, `select sp_id, sector_number as sector_num,ticket_epoch, ticket_value, tree_d_cid as orig_unsealed_cid, tree_r_cid as orig_sealed_cid, precommit_msg_cid as msg_cid_precommit, seed_value, porep_proof from sectors_sdr_pipeline where sector_number=$1`, uint64(sid))
		if err != nil {
			log.Errorf("db2:%+v", err)
			return err
		}
		if len(sectorMetas) != 1 {
			log.Errorw("got from db2", "sectorMetas", len(sectorMetas))
			return err
		}
	}
	sectorMeta := sectorMetas[0]
	log.Infow("got from db", "sectorMeta", sectorMeta)

	pieceSize := abi.PaddedPieceSize(sectorSize).Unpadded()
	pieceCid, err := cid.Decode(sectorMeta.OrigUnsealedCid)
	if err != nil {
		return err
	}

	sealedCid, err := cid.Decode(sectorMeta.OrigSealedCid)
	if err != nil {
		return err
	}

	seed := abi.InteractiveSealRandomness(sectorMeta.Seed)
	pieces := []abi.PieceInfo{
		{
			Size:     pieceSize.Padded(),
			PieceCID: pieceCid,
		},
	}
	sectorCids := storiface.SectorCids{
		Unsealed: pieceCid,
		Sealed:   sealedCid,
	}

	c1out, err := sb.SealCommit1(ctx, sector, abi.SealRandomness(sectorMeta.TicketValue), seed, pieces, sectorCids)
	if err != nil {
		return err
	}

	log.Infow("c1 complete", "c1out", len(c1out))

	// c2
	c2out, err := sb.SealCommit2(ctx, sector, c1out)
	if err != nil {
		return err
	}
	log.Infow("c2 complete", "c2out.len", len(c2out))
	log.Infow("c2 complete", "c2out", c2out)
	log.Infow("c2 complete", "PorepProof", sectorMeta.PorepProof)

	if !bytes.Equal(sectorMeta.PorepProof, []byte(c2out)) {
		log.Info("c2 not equal")
	} else {
		log.Info("c2 Equal")
	}

	return nil
}
