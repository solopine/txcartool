package main

import (
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"os"
)

var log = logging.Logger("tx-car-tool")

func main() { os.Exit(main1()) }

func main1() int {
	app := &cli.App{
		Name:  "car",
		Usage: "Utility for working with car files",
		Commands: []*cli.Command{
			{
				Name:    "create",
				Usage:   "Create a car file",
				Aliases: []string{"c"},
				Action:  CreateCar,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "dest",
						Aliases: []string{"o"},
						Usage:   "The root dir of car file",
					},
					&cli.StringFlag{
						Name:    "key",
						Aliases: []string{"k"},
						Usage:   "The key uuid of car file",
					},
				},
			},
			{
				Name:    "batch",
				Usage:   "batch Create car files",
				Aliases: []string{"b"},
				Action:  BatchCreateCar,
				Flags: []cli.Flag{
					&cli.UintFlag{
						Name:    "count",
						Aliases: []string{"c"},
						Usage:   "count of car",
					},
				},
			},
			{
				Name:   "reseal",
				Usage:  "reseal",
				Action: Reseal,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "seal-dir",
						Usage: "redo sector seal directory",
						Value: "",
					}, &cli.StringFlag{
						Name:  "storage-dir",
						Usage: "the storage directory where the redo sector is stored",
						Value: "",
					}, &cli.StringFlag{
						Name:  "sid-file",
						Usage: "sid-file",
					}, &cli.StringFlag{
						Name:  "p1max",
						Usage: "p1max",
					}, &cli.StringFlag{
						Name:  "smax",
						Usage: "smax",
					}, &cli.StringFlag{
						Name:  "dbhost",
						Usage: "dbhost",
					}, &cli.StringFlag{
						Name:  "dbuser",
						Usage: "dbuser",
					}, &cli.StringFlag{
						Name:  "dbpwd",
						Usage: "dbpwd",
					},
				},
			},
			{
				Name:    "gen-c1",
				Usage:   "gen-c1",
				Aliases: []string{"b"},
				Action:  GenC1,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "seal-dir",
						Usage:    "redo sector seal directory",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "sid",
						Usage:    "sid",
						Required: true,
					}, &cli.StringFlag{
						Name:  "dbhost",
						Usage: "dbhost",
					}, &cli.StringFlag{
						Name:  "dbuser",
						Usage: "dbuser",
					}, &cli.StringFlag{
						Name:  "dbpwd",
						Usage: "dbpwd",
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("err:%+v", err)
		return 1
	}
	return 0
}
