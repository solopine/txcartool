package main

import (
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
)

type TxCarInfo struct {
	CarKey    uuid.UUID
	PieceCid  cid.Cid
	PieceSize int64
	CarSize   int64
}
