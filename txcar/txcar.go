package txcar

import (
	"context"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/solopine/txcartool/lib/filestore"
	"github.com/solopine/txcartool/lib/shared"
	"github.com/solopine/txcartool/txcar/v1"
	"github.com/solopine/txcartool/txcar/v2"
	"golang.org/x/xerrors"
	"os"
	"path"
)

var log = logging.Logger("txcar")

const (
	SupportedSectorSize          = abi.PaddedPieceSize(32 << 30)
	SupportedRegisteredSealProof = abi.RegisteredSealProof_StackedDrg32GiBV1_1
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

	randFile := tc.key.String() + "." + uuid.New().String() + ".car"

	destFile := path.Join(destDir, randFile)

	err = tc.builder.CreateCarFile(ctx, tc.key, destFile)
	if err != nil {
		return "", nil, err
	}

	piece, err := genTxPiece(destFile)
	if err != nil {
		return "", nil, err
	}

	if piece.PieceSize != SupportedSectorSize {
		return "", nil, xerrors.Errorf("piece.PieceSize(%w) != SupportedSectorSize", piece.PieceSize)
	}

	return destFile, piece, nil
}
func genTxPiece(txCarFile string) (*TxPiece, error) {
	dataCIDSize, err := GenCommp(txCarFile)
	if err != nil {
		return nil, err
	}
	piece := TxPiece{
		PieceCid:  dataCIDSize.PieceCID,
		PieceSize: dataCIDSize.PieceSize,
		CarSize:   abi.UnpaddedPieceSize(dataCIDSize.PayloadSize),
	}
	return &piece, nil
}

type TxCarBuilder interface {
	CreateCarFile(ctx context.Context, key uuid.UUID, destFile string) error
}

func GenUnsealedFile(ctx context.Context, piece TxPiece, carFile string) (string, abi.PieceInfo, error) {

	// maker reader
	dcfs, err := filestore.NewLocalFileStore("/")
	if err != nil {
		return "", abi.PieceInfo{}, err
	}

	dcFile, err := dcfs.Open(filestore.Path(carFile))
	if err != nil {
		return "", abi.PieceInfo{}, err
	}

	paddedReader, err := shared.NewInflatorReader(dcFile, uint64(dcFile.Size()), piece.PieceSize.Unpadded())
	if err != nil {
		return "", abi.PieceInfo{}, err
	}

	//
	unsealedFile, pi, err := AddPiece(ctx, piece.PieceSize.Unpadded(), paddedReader, carFile)
	if err != nil {
		log.Errorw("AddPiece error", "err", err)
		return "", abi.PieceInfo{}, err
	}
	log.Infow("AddPiece", "pi", pi)

	if pi.PieceCID != piece.PieceCid {
		log.Errorw("pi.PieceCID != piece.PieceCid", "pi.PieceCID", pi.PieceCID.String(), "piece.PieceCid", piece.PieceCid.String())
		return "", abi.PieceInfo{}, err
	}
	return unsealedFile, pi, nil
}
