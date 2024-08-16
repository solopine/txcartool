package main

import (
	"fmt"
	lcli "github.com/filecoin-project/lotus/cli"
	lotus_repo "github.com/filecoin-project/lotus/node/repo"
	"github.com/solopine/txcartool/lib/boost/node"
	"github.com/solopine/txcartool/lib/boost/node/modules/dtypes"

	"github.com/urfave/cli/v2"
)

const (
	FlagBoostRepo = "tx-repo"
)

func IndexCar(cctx *cli.Context) error {

	//subCh := gateway.NewEthSubHandler()
	//fullnodeApi, ncloser, err := lcli.GetFullNodeAPIV1(cctx, lcliutil.FullNodeWithEthSubscribtionHandler(subCh))
	//if err != nil {
	//	return fmt.Errorf("getting full node api: %w", err)
	//}
	//defer ncloser()

	ctx := lcli.ReqContext(cctx)

	boostRepoPath := cctx.String(FlagBoostRepo)

	r, err := lotus_repo.NewFS(boostRepoPath)
	if err != nil {
		return err
	}
	ok, err := r.Exists()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("repo at '%s' is not initialized", cctx.String(FlagBoostRepo))
	}

	shutdownChan := make(chan struct{})

	stop, err := node.New(ctx,
		node.Override(new(dtypes.ShutdownChan), shutdownChan),
		node.Base(),
		node.Repo(r),
	)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}

	// Monitor for shutdown.
	finishCh := node.MonitorShutdown(shutdownChan,
		node.ShutdownHandler{Component: "boost", StopFunc: stop},
	)

	<-finishCh

	////////////
	////ctx := c.Context
	////txCarVersion := txcar.TxCarVersion(c.Uint("version"))
	//
	//carFile := c.String("car-file")
	//
	//_, err := os.Stat(carFile)
	//if err != nil {
	//	return err
	//}
	//
	//f, err := os.Open(carFile)
	//if err != nil {
	//	return err
	//}
	//records, err := txcar.ParseRecordsFromCar(f)
	//if err != nil {
	//	return err
	//}
	//
	//for i := 0; i < 5; i++ {
	//	cid := records[i].Cid
	//	mh := cid.Hash().String()
	//	log.Infow("IndexCar", "i", i, "cid", cid.String(), "mh", mh, "cid.version", cid.Version(), "cid.Type", cid.Type())
	//}
	//
	//recordsLen := len(records)
	//for i := 0; i < 5; i++ {
	//	cid := records[recordsLen-6+i].Cid
	//	mh := cid.Hash().String()
	//	log.Infow("IndexCar", "i", recordsLen-6+i, "cid", cid.String(), "mh", mh, "cid.version", cid.Version(), "cid.Type", cid.Type())
	//}

	return nil
}
