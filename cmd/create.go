package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/solopine/txcartool/txcar"
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

	txCar := txcar.NewTxCar(txCarVersion, key)
	carFile, txPiece, err := txCar.CreateCarFile(ctx)
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

	for i := 0; i < count; i++ {
		key := uuid.New()

		txCar := txcar.NewTxCar(txCarVersion, key)
		destFile, txPiece, err := txCar.CreateCarFile(c.Context)
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
