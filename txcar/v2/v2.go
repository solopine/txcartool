package v2

import (
	"context"
	"github.com/google/uuid"
)

type TxCarBuilderV2 struct {
}

func (TxCarBuilderV2) CreateCarFile(ctx context.Context, key uuid.UUID, destFile string) error {
	return nil
}
