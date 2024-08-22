package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/solopine/txcar/txcar"
	"github.com/urfave/cli/v2"
	"os"
)

// CreateCar creates a txcar
func CreateCar(c *cli.Context) error {
	ctx := c.Context
	txCarVersion := txcar.TxCarVersion(c.Uint("version"))
	unsealed := c.Bool("unsealed")

	var err error
	var key uuid.UUID
	if !c.IsSet("key") {
		key = uuid.New()
	} else {
		keyStr := c.String("key")
		key, err = uuid.Parse(keyStr)
		if err != nil {
			return fmt.Errorf("key is not uuid")
		}
	}

	carFile, txPiece, err := txcar.CreateCarFile(ctx, key, txCarVersion)
	if err != nil {
		return err
	}

	fmt.Printf("carFile:%s\n", carFile)
	fmt.Printf("%s\t%s\t%d\t%d\n", key.String(), txPiece.PieceCid.String(), txPiece.PieceSize, txPiece.CarSize)

	if unsealed {
		// create unsealed file
		unsealedFile, _, err := txcar.GenUnsealedFile(ctx, *txPiece, carFile)
		if err != nil {
			return err
		}
		fmt.Printf("unsealedFile:%s\n", unsealedFile)
	}

	return nil
}

// BatchCreateCar
func BatchCreateCar(c *cli.Context) error {

	txCarVersion := txcar.TxCarVersion(c.Uint("version"))
	count := int(c.Uint("count"))
	ctx := c.Context

	for i := 0; i < count; i++ {
		key := uuid.New()

		destFile, txPiece, err := txcar.CreateCarFile(ctx, key, txCarVersion)
		if err != nil {
			return err
		}

		err = os.Remove(destFile)
		if err != nil {
			return err
		}

		fmt.Printf("%s\t%s\t%d\t%d\n", key.String(), txPiece.PieceCid.String(), txPiece.PieceSize, txPiece.CarSize)
	}
	return nil
}
