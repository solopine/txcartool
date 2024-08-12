package txcar

import (
	"context"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/solopine/txcartool/lib"
	"github.com/solopine/txcartool/txcar/v1"
	"github.com/solopine/txcartool/txcar/v2"
	"os"
	"path"
)

type TxCarVersion int

const (
	TxCarV1 TxCarVersion = 1
	TxCarV2 TxCarVersion = 2
)

var TxCarBuilders = map[TxCarVersion]TxCarBuilder{
	TxCarV1: &v1.TxCarBuilderV1{},
	TxCarV2: &v2.TxCarBuilderV2{},
}

type TxCar struct {
	version TxCarVersion
	key     uuid.UUID
	builder TxCarBuilder
}

type TxPiece struct {
	PieceCid  cid.Cid
	PieceSize abi.PaddedPieceSize
	CarSize   abi.UnpaddedPieceSize
}

func NewTxCar(version TxCarVersion, key uuid.UUID) *TxCar {
	return &TxCar{
		version: version,
		key:     key,
		builder: TxCarBuilders[version],
	}
}

func (tc TxCar) CreateCarFile(ctx context.Context) (string, *TxPiece, error) {

	destDir := "/cartmp"
	_, err := os.Stat(destDir)
	if err != nil {
		destDir = os.TempDir()
	}

	randFile := tc.key.String() + "." + uuid.New().String() + ".txcar"

	destFile := path.Join(destDir, randFile)

	err = tc.builder.CreateCarFile(ctx, tc.key, destFile)
	if err != nil {
		return "", nil, err
	}

	piece, err := genTxPiece(destFile)
	if err != nil {
		return "", nil, err
	}

	return destFile, piece, nil
}
func genTxPiece(txCarFile string) (*TxPiece, error) {
	pieceCid, pieceSize, carSize, err := lib.GenCommp(txCarFile)
	if err != nil {
		return nil, err
	}
	piece := TxPiece{
		PieceCid:  pieceCid,
		PieceSize: abi.PaddedPieceSize(pieceSize),
		CarSize:   abi.UnpaddedPieceSize(carSize),
	}
	return &piece, nil
}

type TxCarBuilder interface {
	CreateCarFile(ctx context.Context, key uuid.UUID, destFile string) error
}
