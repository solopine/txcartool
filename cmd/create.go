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

	txCar := txcar.NewTxCar(txcar.TxCarV1, key)
	destFile, txPiece, err := txCar.CreateCarFile(c.Context)
	if err != nil {
		return err
	}

	fmt.Printf("destFile:%s\n", destFile)
	fmt.Printf("%s\t%s\t%d\t%d\n", key.String(), txPiece.PieceCid.String(), txPiece.PieceSize, txPiece.CarSize)

	return nil
}

// BatchCreateCar
func BatchCreateCar(c *cli.Context) error {

	if !c.IsSet("count") {
		return fmt.Errorf("count is missing")
	}
	count := int(c.Uint("count"))

	for i := 0; i < count; i++ {
		key := uuid.New()

		txCar := txcar.NewTxCar(txcar.TxCarV1, key)
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
