package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

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
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Println(err)
		return 1
	}
	return 0
}