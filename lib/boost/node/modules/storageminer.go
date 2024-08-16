package modules

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/solopine/txcartool/lib/boost/db"
	"github.com/solopine/txcartool/lib/boost/markets/sectoraccessor"
	"github.com/solopine/txcartool/lib/boost/node/config"
	"github.com/solopine/txcartool/lib/boostd-data/shared/tracing"
	"path"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/account"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api/v1api"
	ctypes "github.com/filecoin-project/lotus/chain/types"
	ltypes "github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/node/repo"
	lotus_repo "github.com/filecoin-project/lotus/node/repo"
	"go.uber.org/fx"
	"go.uber.org/multierr"
)

var (
	StorageCounterDSPrefix = "/storage/nextid"
)

func readCfg(r lotus_repo.LockedRepo, accessor func(*config.Boost)) error {
	raw, err := r.Config()
	if err != nil {
		return err
	}

	cfg, ok := raw.(*config.Boost)
	if !ok {
		return errors.New("expected address of config.Boost")
	}

	accessor(cfg)

	return nil
}

func mutateCfg(r lotus_repo.LockedRepo, mutator func(*config.Boost)) error {
	var typeErr error

	setConfigErr := r.SetConfig(func(raw interface{}) {
		cfg, ok := raw.(*config.Boost)
		if !ok {
			typeErr = errors.New("expected boost config")
			return
		}

		mutator(cfg)
	})

	return multierr.Combine(typeErr, setConfigErr)
}

func NewBoostDB(r lotus_repo.LockedRepo) (*sql.DB, error) {
	// fixes error "database is locked", caused by concurrent access from deal goroutines to a single sqlite3 db connection
	// see: https://github.com/mattn/go-sqlite3#:~:text=Error%3A%20database%20is%20locked
	dbPath := path.Join(r.Path(), db.DealsDBName+"?cache=shared")
	return db.SqlDB(dbPath)
}

type LogSqlDB struct {
	db *sql.DB
}

func NewLogsSqlDB(r repo.LockedRepo) (*LogSqlDB, error) {
	// fixes error "database is locked", caused by concurrent access from deal goroutines to a single sqlite3 db connection
	// see: https://github.com/mattn/go-sqlite3#:~:text=Error%3A%20database%20is%20locked
	dbPath := path.Join(r.Path(), db.LogsDBName+"?cache=shared")
	d, err := db.SqlDB(dbPath)
	if err != nil {
		return nil, err
	}
	return &LogSqlDB{d}, nil
}

func NewDirectDealsDB(sqldb *sql.DB) *db.DirectDealsDB {
	return db.NewDirectDealsDB(sqldb)
}

func NewSectorStateDB(sqldb *sql.DB) *db.SectorStateDB {
	return db.NewSectorStateDB(sqldb)
}

type signatureVerifier struct {
	fn v1api.FullNode
}

func (s *signatureVerifier) VerifySignature(ctx context.Context, sig crypto.Signature, addr address.Address, input []byte) (bool, error) {
	addr, err := s.fn.StateAccountKey(ctx, addr, ctypes.EmptyTSK)
	if err != nil {
		return false, err
	}

	// Check if the client is an f4 address, ie an FVM contract
	clientAddr := addr.String()
	if len(clientAddr) >= 2 && (clientAddr[:2] == "t4" || clientAddr[:2] == "f4") {
		// Verify authorization by simulating an AuthenticateMessage
		return s.verifyContractSignature(ctx, sig, addr, input)
	}

	// Otherwise do local signature verification
	err = sigs.Verify(&sig, addr, input)
	return err == nil, err
}

// verifyContractSignature simulates sending an AuthenticateMessage to authenticate the signer
func (s *signatureVerifier) verifyContractSignature(ctx context.Context, sig crypto.Signature, addr address.Address, input []byte) (bool, error) {
	var params account.AuthenticateMessageParams
	params.Message = input
	params.Signature = sig.Data

	var msg ltypes.Message
	buf := new(bytes.Buffer)

	var err error
	err = params.MarshalCBOR(buf)
	if err != nil {
		return false, err
	}
	msg.Params = buf.Bytes()

	msg.From = builtin.StorageMarketActorAddr
	msg.To = addr
	msg.Nonce = 1

	msg.Method, err = builtin.GenerateFRCMethodNum("AuthenticateMessage") // abi.MethodNum(2643134072)
	if err != nil {
		return false, err
	}

	res, err := s.fn.StateCall(ctx, &msg, ltypes.EmptyTSK)
	if err != nil {
		return false, fmt.Errorf("state call to %s returned an error: %w", addr, err)
	}

	return res.MsgRct.ExitCode == exitcode.Ok, nil
}

// Use a caching sector accessor
func NewSectorAccessor(cfg *config.Boost) sectoraccessor.SectorAccessorConstructor {
	// The cache just holds booleans, so there's no harm in using a big number
	// for cache size
	const maxCacheSize = 4096
	return sectoraccessor.NewCachingSectorAccessor(maxCacheSize, time.Duration(cfg.Dealmaking.IsUnsealedCacheExpiry))
}

func NewTracing(cfg *config.Boost) func(lc fx.Lifecycle) (*tracing.Tracing, error) {
	return func(lc fx.Lifecycle) (*tracing.Tracing, error) {
		if cfg.Tracing.Enabled {
			// Instantiate the tracer and exporter.go
			stop, err := tracing.New(cfg.Tracing.ServiceName, cfg.Tracing.Endpoint)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate tracer: %w", err)
			}
			lc.Append(fx.Hook{
				OnStop: stop,
			})
		}

		return &tracing.Tracing{}, nil
	}
}
