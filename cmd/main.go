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
				Action: CreateCar,
				Flags: []cli.Flag{
					&cli.UintFlag{
						Name:    "count",
						Aliases: []string{"c"},
						Usage:   "count of car",
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
