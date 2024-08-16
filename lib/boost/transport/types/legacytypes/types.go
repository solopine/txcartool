package legacytypes

import (
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
)

var log = logging.Logger("storagemrkt")

//go:generate cbor-gen-for --map-encoding ClientDeal MinerDeal Balance SignedStorageAsk StorageAsk DataRef ProviderDealState DealStages DealStage Log

// The ID for the libp2p protocol for proposing storage deals.
const DealProtocolID101 = "/fil/storage/mk/1.0.1"
const DealProtocolID110 = "/fil/storage/mk/1.1.0"
const DealProtocolID111 = "/fil/storage/mk/1.1.1"

// AskProtocolID is the ID for the libp2p protocol for querying miners for their current StorageAsk.
const OldAskProtocolID = "/fil/storage/ask/1.0.1"
const AskProtocolID = "/fil/storage/ask/1.1.0"

// DealStatusProtocolID is the ID for the libp2p protocol for querying miners for the current status of a deal.
const OldDealStatusProtocolID = "/fil/storage/status/1.0.1"
const DealStatusProtocolID = "/fil/storage/status/1.1.0"

// Balance represents a current balance of funds in the StorageMarketActor.
type Balance struct {
	Locked    abi.TokenAmount
	Available abi.TokenAmount
}

// StorageAsk defines the parameters by which a miner will choose to accept or
// reject a deal. Note: making a storage deal proposal which matches the miner's
// ask is a precondition, but not sufficient to ensure the deal is accepted (the
// storage provider may run its own decision logic).
type StorageAsk struct {
	// Price per GiB / Epoch
	Price         abi.TokenAmount
	VerifiedPrice abi.TokenAmount

	MinPieceSize abi.PaddedPieceSize
	MaxPieceSize abi.PaddedPieceSize
	Miner        address.Address
	Timestamp    abi.ChainEpoch
	Expiry       abi.ChainEpoch
	SeqNo        uint64
}

// SignedStorageAsk is an ask signed by the miner's private key
type SignedStorageAsk struct {
	Ask       *StorageAsk
	Signature *crypto.Signature
}

// SignedStorageAskUndefined represents the empty value for SignedStorageAsk
var SignedStorageAskUndefined = SignedStorageAsk{}

// StorageAskOption allows custom configuration of a storage ask
type StorageAskOption func(*StorageAsk)

// MinPieceSize configures a minimum piece size of a StorageAsk
func MinPieceSize(minPieceSize abi.PaddedPieceSize) StorageAskOption {
	return func(sa *StorageAsk) {
		sa.MinPieceSize = minPieceSize
	}
}

// MaxPieceSize configures maxiumum piece size of a StorageAsk
func MaxPieceSize(maxPieceSize abi.PaddedPieceSize) StorageAskOption {
	return func(sa *StorageAsk) {
		sa.MaxPieceSize = maxPieceSize
	}
}

// StorageAskUndefined represents an empty value for StorageAsk
var StorageAskUndefined = StorageAsk{}

type ClientDealProposal = market.ClientDealProposal

// NewDealStages creates a new DealStages object ready to be used.
// EXPERIMENTAL; subject to change.
func NewDealStages() *DealStages {
	return &DealStages{}
}

// DealStages captures a timeline of the progress of a deal, grouped by stages.
// EXPERIMENTAL; subject to change.
type DealStages struct {
	// Stages contains an entry for every stage that the deal has gone through.
	// Each stage then contains logs.
	Stages []*DealStage
}

// DealStages captures data about the execution of a deal stage.
// EXPERIMENTAL; subject to change.
type DealStage struct {
	// Human-readable fields.
	// TODO: these _will_ need to be converted to canonical representations, so
	//  they are machine readable.
	Name             string
	Description      string
	ExpectedDuration string

	// Timestamps.
	// TODO: may be worth adding an exit timestamp. It _could_ be inferred from
	//  the start of the next stage, or from the timestamp of the last log line
	//  if this is a terminal stage. But that's non-determistic and it relies on
	//  assumptions.
	CreatedTime cbg.CborTime
	UpdatedTime cbg.CborTime

	// Logs contains a detailed timeline of events that occurred inside
	// this stage.
	Logs []*Log
}

// Log represents a point-in-time event that occurred inside a deal stage.
// EXPERIMENTAL; subject to change.
type Log struct {
	// Log is a human readable message.
	//
	// TODO: this _may_ need to be converted to a canonical data model so it
	//  is machine-readable.
	Log string

	UpdatedTime cbg.CborTime
}

// GetStage returns the DealStage object for a named stage, or nil if not found.
//
// TODO: the input should be a strongly-typed enum instead of a free-form string.
// TODO: drop Get from GetStage to make this code more idiomatic. Return a
// second ok boolean to make it even more idiomatic.
// EXPERIMENTAL; subject to change.
func (ds *DealStages) GetStage(stage string) *DealStage {
	if ds == nil {
		return nil
	}

	for _, s := range ds.Stages {
		if s.Name == stage {
			return s
		}
	}

	return nil
}

// StorageProviderInfo describes on chain information about a StorageProvider
// (use QueryAsk to determine more specific deal parameters)
type StorageProviderInfo struct {
	Address    address.Address // actor address
	Owner      address.Address
	Worker     address.Address // signs messages
	SectorSize uint64
	PeerID     peer.ID
	Addrs      []ma.Multiaddr
}

// ProposeStorageDealResult returns the result for a proposing a deal
type ProposeStorageDealResult struct {
	ProposalCid cid.Cid
}

// ProposeStorageDealParams describes the parameters for proposing a storage deal
type ProposeStorageDealParams struct {
	Addr          address.Address
	Info          *StorageProviderInfo
	Data          *DataRef
	StartEpoch    abi.ChainEpoch
	EndEpoch      abi.ChainEpoch
	Price         abi.TokenAmount
	Collateral    abi.TokenAmount
	Rt            abi.RegisteredSealProof
	FastRetrieval bool
	VerifiedDeal  bool
}

const (
	// TTGraphsync means data for a deal will be transferred by graphsync
	TTGraphsync = "graphsync"

	// TTManual means data for a deal will be transferred manually and imported
	// on the provider
	TTManual = "manual"
)

// DataRef is a reference for how data will be transferred for a given storage deal
type DataRef struct {
	TransferType string
	Root         cid.Cid

	PieceCid     *cid.Cid              // Optional for non-manual transfer, will be recomputed from the data if not given
	PieceSize    abi.UnpaddedPieceSize // Optional for non-manual transfer, will be recomputed from the data if not given
	RawBlockSize uint64                // Optional: used as the denominator when calculating transfer %
}
