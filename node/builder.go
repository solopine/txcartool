package node

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	lotus_journal "github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/alerting"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	lotus_modules "github.com/filecoin-project/lotus/node/modules"
	lotus_dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	lotus_helpers "github.com/filecoin-project/lotus/node/modules/helpers"
	lotus_repo "github.com/filecoin-project/lotus/node/repo"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-metrics-interface"
	"github.com/multiformats/go-multihash"
	"github.com/solopine/txcar/txcar"
	"github.com/solopine/txcartool/lib/boost/build"
	"github.com/solopine/txcartool/lib/boost/cmd/lib"
	"github.com/solopine/txcartool/lib/boost/db"
	"github.com/solopine/txcartool/lib/boost/node/config"
	"github.com/solopine/txcartool/lib/boost/node/modules"
	"github.com/solopine/txcartool/lib/boost/node/repo"
	"github.com/solopine/txcartool/lib/boost/piecedirectory"
	bdclient "github.com/solopine/txcartool/lib/boostd-data/client"
	"github.com/solopine/txcartool/lib/boostd-data/model"
	"github.com/yugabyte/gocql"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

//nolint:deadcode,varcheck
var log = logging.Logger("builder")
var fxlog = logging.Logger("fxlog")

// special is a type used to give keys to modules which
//
//	can't really be identified by the returned type
type special struct{ id int }

//nolint:golint
var (
	DefaultTransportsKey = special{0}  // Libp2p option
	DiscoveryHandlerKey  = special{2}  // Private type
	AddrsFactoryKey      = special{3}  // Libp2p option
	SmuxTransportKey     = special{4}  // Libp2p option
	RelayKey             = special{5}  // Libp2p option
	SecurityKey          = special{6}  // Libp2p option
	BaseRoutingKey       = special{7}  // fx groups + multiret
	NatPortMapKey        = special{8}  // Libp2p option
	ConnectionManagerKey = special{9}  // Libp2p option
	AutoNATSvcKey        = special{10} // Libp2p option
	BandwidthReporterKey = special{11} // Libp2p option
	ConnGaterKey         = special{12} // libp2p option
	ResourceManagerKey   = special{14} // Libp2p option
	UserAgentKey         = special{15} // Libp2p option
)

type invoke int

// Invokes are called in the order they are defined.
//
//nolint:golint
const (
	// InitJournal at position 0 initializes the journal global var as soon as
	// the system starts, so that it's available for all other components.
	InitJournalKey = invoke(iota)

	// health checks
	CheckFDLimit
	StartJobKey

	_nInvokes // keep this last
)

type Settings struct {
	// modules is a map of constructors for DI
	//
	// In most cases the index will be a reflect. Type of element returned by
	// the constructor, but for some 'constructors' it's hard to specify what's
	// the return type should be (or the constructor returns fx group)
	modules map[interface{}]fx.Option

	// invokes are separate from modules as they can't be referenced by return
	// type, and must be applied in correct order
	invokes []fx.Option

	nodeType lotus_repo.RepoType

	Base   bool // Base option applied
	Config bool // Config option applied
	Lite   bool // Start node in "lite" mode
}

// Basic lotus-app services
func defaults() []Option {
	return []Option{
		// global system journal
		Override(new(lotus_journal.DisabledEvents), lotus_journal.EnvDisabledEvents),
		Override(new(lotus_journal.Journal), lotus_modules.OpenFilesystemJournal),
		Override(new(*alerting.Alerting), alerting.NewAlertingSystem),
		//Override(new(lotus_dtypes.NodeStartTime), FromVal(lotus_dtypes.NodeStartTime(time.Now()))),
		//
		Override(CheckFDLimit, lotus_modules.CheckFdLimit(build.DefaultFDLimit)),
		//
		//Override(new(system.MemoryConstraints), modules.MemoryConstraints),
		//
		Override(new(lotus_helpers.MetricsCtx), func() context.Context {
			return metrics.CtxScope(context.Background(), "boost")
		}),

		//Override(new(lotus_dtypes.ShutdownChan), make(chan struct{})),
	}
}

func Base() Option {
	return Options(
		func(s *Settings) error { s.Base = true; return nil }, // mark Base as applied
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the Base() option must be set before Config option")),
		),
		BoostNode,
	)
}

// Config sets up constructors based on the provided Config
func ConfigCommon(cfg *config.Common) Option {
	return Options(
		func(s *Settings) error { s.Config = true; return nil },
		//Override(new(dtypes.APIEndpoint), func() (dtypes.APIEndpoint, error) {
		//	return multiaddr.NewMultiaddr(cfg.API.ListenAddress)
		//}),
		//Override(SetApiEndpointKey, func(lr lotus_repo.LockedRepo, e dtypes.APIEndpoint) error {
		//	return lr.SetAPIEndpoint(e)
		//}),
		//Override(new(paths.URLs), func(e dtypes.APIEndpoint) (paths.URLs, error) {
		//	ip := cfg.API.RemoteListenAddress
		//
		//	var urls paths.URLs
		//	urls = append(urls, "http://"+ip+"/remote") // TODO: This makes no assumptions, and probably could...
		//	return urls, nil
		//}),
		//ApplyIf(func(s *Settings) bool { return s.Base }), // apply only if Base has already been applied
		//Override(new(api.Net), From(new(lotus_net.NetAPI))),
		//
		//Override(new(lotus_api.Net), From(new(lotus_net.NetAPI))),
		//Override(new(lotus_api.Common), From(new(lotus_common.CommonAPI))),

		Override(new(lotus_dtypes.MetadataDS), lotus_modules.Datastore(cfg.Backup.DisableMetadataLog)),
		//Override(StartListeningKey, lotus_lp2p.StartListening(cfg.Libp2p.ListenAddresses)),
		//Override(ConnectionManagerKey, lotus_lp2p.ConnectionManager(
		//	cfg.Libp2p.ConnMgrLow,
		//	cfg.Libp2p.ConnMgrHigh,
		//	time.Duration(cfg.Libp2p.ConnMgrGrace),
		//	cfg.Libp2p.ProtectedPeers)),
		//ApplyIf(func(s *Settings) bool { return len(cfg.Libp2p.BootstrapPeers) > 0 },
		//	Override(new(lotus_dtypes.BootstrapPeers), modules.ConfigBootstrap(cfg.Libp2p.BootstrapPeers)),
		//),
		//
		////Override(new(network.ResourceManager), modules.ResourceManager(cfg.Libp2p.ConnMgrHigh)),
		////Override(ResourceManagerKey, lp2p.ResourceManagerOption),
		////Override(new(*pubsub.PubSub), lp2p.GossipSub),
		////Override(new(*lotus_config.Pubsub), &cfg.Pubsub),
		//
		//ApplyIf(func(s *Settings) bool { return len(cfg.Libp2p.BootstrapPeers) > 0 },
		//	Override(new(lotus_dtypes.BootstrapPeers), modules.ConfigBootstrap(cfg.Libp2p.BootstrapPeers)),
		//),
		//
		//Override(AddrsFactoryKey, lotus_lp2p.AddrsFactory(
		//	cfg.Libp2p.AnnounceAddresses,
		//	cfg.Libp2p.NoAnnounceAddresses)),
		//If(!cfg.Libp2p.DisableNatPortMap, Override(NatPortMapKey, lotus_lp2p.NatPortMap)),
	)
}

func Repo(r lotus_repo.Repo) Option {
	return func(settings *Settings) error {
		lr, err := r.Lock(settings.nodeType)
		if err != nil {
			return err
		}
		// If it's not a mem-repo
		if _, ok := r.(*lotus_repo.MemRepo); !ok {
			// Migrate config file
			err = config.ConfigMigrate(lr.Path())
			if err != nil {
				return fmt.Errorf("migrating config: %w", err)
			}
		}
		c, err := lr.Config()
		if err != nil {
			return err
		}
		cfg, ok := c.(*config.Boost)
		if !ok {
			return fmt.Errorf("invalid config type from repo, expected *config.Boost but got %T", c)
		}

		return Options(
			Override(new(lotus_repo.LockedRepo), lotus_modules.LockedRepo(lr)), // module handles closing
			//
			//Override(new(ci.PrivKey), lotus_lp2p.PrivKey),
			//Override(new(ci.PubKey), ci.PrivKey.GetPublic),
			//Override(new(peer.ID), peer.IDFromPublicKey),
			//
			//Override(new(types.KeyStore), modules.KeyStore),
			//
			//Override(new(*lotus_dtypes.APIAlg), lotus_modules.APISecret),

			ConfigBoost(cfg),
		)(settings)
	}
}

type StopFunc func(context.Context) error

// New builds and starts new Filecoin node
func New(ctx context.Context, opts ...Option) (StopFunc, error) {
	settings := Settings{
		modules: map[interface{}]fx.Option{},
		invokes: make([]fx.Option, _nInvokes),
	}

	// apply module options in the right order
	if err := Options(Options(defaults()...), Options(opts...))(&settings); err != nil {
		return nil, fmt.Errorf("applying node options failed: %w", err)
	}

	// gather constructors for fx.Options
	ctors := make([]fx.Option, 0, len(settings.modules))
	for _, opt := range settings.modules {
		ctors = append(ctors, opt)
	}

	// fill holes in invokes for use in fx.Options
	for i, opt := range settings.invokes {
		if opt == nil {
			settings.invokes[i] = fx.Options()
		}
	}

	app := fx.New(
		fx.Options(ctors...),
		fx.Options(settings.invokes...),

		fx.WithLogger(func() fxevent.Logger {
			return &fxevent.ZapLogger{Logger: fxlog.Desugar()}
		}),
	)

	// TODO: we probably should have a 'firewall' for Closing signal
	//  on this context, and implement closing logic through lifecycles
	//  correctly
	if err := app.Start(ctx); err != nil {
		// comment fx.NopLogger few lines above for easier debugging
		return nil, fmt.Errorf("starting node: %w", err)
	}

	return app.Stop, nil
}

var BoostNode = Options(
	//Override(new(sealer.StorageAuth), lotus_modules.StorageAuth),

	// Actor config
	Override(new(lotus_dtypes.MinerAddress), lotus_modules.MinerAddress),
	Override(new(lotus_dtypes.MinerID), lotus_modules.MinerID),
	//
	//Override(new(lotus_dtypes.NetworkName), lotus_modules.StorageNetworkName),
	Override(new(*sql.DB), modules.NewBoostDB),
	//Override(new(*modules.LogSqlDB), modules.NewLogsSqlDB),
	Override(new(*db.DirectDealsDB), modules.NewDirectDealsDB),
	//Override(new(*db.SectorStateDB), modules.NewSectorStateDB),
)

func ConfigBoost(cfg *config.Boost) Option {

	return Options(
		ConfigCommon(&cfg.Common),

		// Lotus Markets (retrieval deps)
		Override(new(*bdclient.Store), modules.NewPieceDirectoryStore(cfg)),
		Override(new(*lib.MultiMinerAccessor), modules.NewMultiminerSectorAccessor(cfg)),
		Override(new(*piecedirectory.PieceDirectory), modules.NewPieceDirectory(cfg)),
		Override(StartJobKey, startJob()),
	)
}

func BoostAPI() Option {
	return Options(
		ApplyIf(func(s *Settings) bool { return s.Config },
			Error(errors.New("the StorageMiner option must be set before Config option")),
		),

		func(s *Settings) error {
			s.nodeType = repo.Boost
			return nil
		},
	)
}

func startJob() func(lc fx.Lifecycle, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {
	return func(lc fx.Lifecycle, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {
		pdctx, cancel := context.WithCancel(context.Background())

		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return doJob3(pdctx, db, pd)
			},
			OnStop: func(ctx context.Context) error {
				cancel()
				return nil
			},
		})
		return nil
	}
}

func doJob(ctx context.Context, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {

	//TX_CAR_KEY/1/3e5f5972-5c5e-410f-8755-dd135149cd1e/baga6ea4seaqmhl6rfrlrl6lpzifv6whwqerv3czmu457w55ipta2dgtvcrqm6dy/34359738368/17185740416
	carKey, err := uuid.Parse("3e5f5972-5c5e-410f-8755-dd135149cd1e")
	if err != nil {
		return err
	}

	id, err := uuid.Parse("a1c9f6e2-027d-490a-83b5-ee23e24147cf")
	//id, err := uuid.Parse("eaadb87e-0281-446d-8b71-344993299869")
	if err != nil {
		return err
	}

	deal, err := db.ByID(ctx, id)
	if err != nil {
		return err
	}

	log.Infow("doJob", "deal", deal)

	entry := deal

	recs, err := pd.ParseRecordsForPiece(ctx, entry.PieceCID, model.DealInfo{
		DealUuid:     entry.ID.String(),
		ChainDealID:  abi.DealID(entry.AllocationID), // Convert the type to avoid migration as underlying types are same
		MinerAddr:    entry.Provider,
		SectorID:     entry.SectorID,
		PieceOffset:  entry.Offset,
		PieceLength:  entry.Length,
		CarLength:    uint64(entry.InboundFileSize),
		IsDirectDeal: true,
	})
	if err != nil {
		return err
	}

	for i, rec := range recs {

		log.Infow("process", "i", i)
		if rec.Cid.Type() == 0x55 {
			//if i == 0 || i == 1 || i == recssize-5 {
			//	log.Infow("skip", "i", i, "cid", rec.Cid.String())
			//	continue
			//}

			block, err := txcar.TxBlockGet(ctx, carKey, rec.OffsetSize.Offset, rec.OffsetSize.Size, txcar.TxCarV1)
			if err != nil {
				return err
			}

			chkc, err := rec.Cid.Prefix().Sum(block)
			if err != nil {
				return err
			}

			log.Infow("hash.sum", "i", i, "chkc", chkc.String(), "rec.Cid", rec.Cid.String())

			if !chkc.Equals(rec.Cid) {

				return err
			}
		}
	}

	//{
	//	cluster := gocql.NewCluster("10.0.3.47")
	//	cluster.Timeout = 60 * time.Second
	//	cluster.Keyspace = "idx"
	//
	//	session, err := cluster.CreateSession()
	//	if err != nil {
	//		return err
	//	}
	//
	//	recssize := len(recs)
	//	for i, rec := range recs {
	//
	//		log.Infow("process", "i", i)
	//		if rec.Cid.Type() == 0x55 {
	//			if i == 0 || i == 1 || i == recssize-5 {
	//				log.Infow("skip", "i", i, "cid", rec.Cid.String())
	//				continue
	//			}
	//			qry := `INSERT INTO txpieceblockoffsetsizev1(payloadmultihash, blockoffset, blocksize) VALUES (?, ?, ?)`
	//			err := session.Query(qry, rec.Cid.Hash(), rec.OffsetSize.Offset, rec.OffsetSize.Size).WithContext(ctx).Exec()
	//			if err != nil {
	//				return err
	//			}
	//
	//		} else if rec.Cid.Type() == 0x70 {
	//			fmt.Printf("%s,%s,%d,%d\n", rec.Cid.String(), rec.Cid.Hash().String(), rec.OffsetSize.Offset, rec.OffsetSize.Size)
	//		} else {
	//			log.Infow("---------other", "cid", rec.Cid.String())
	//		}
	//	}
	//
	//}
	log.Infow("dddd", "len", len(recs))
	return nil

	//{
	//	f, err := os.Open("/cartmp/3e5f5972-5c5e-410f-8755-dd135149cd1e.1894b5dc-0195-4abe-a098-be373f50e5fe.car")
	//	if err != nil {
	//		return err
	//	}
	//	rs, err := piecedirectory.TxParseRecordsFromCar(f)
	//	if err != nil {
	//		return err
	//	}
	//
	//	log.Infow("dddd", "rs", len(rs))
	//
	//}

	return nil
}

func doJob2(ctx context.Context, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {
	path := "/cu1store2/cache/s-t03143698-1664"
	sz, err := FileSize(path)
	if err != nil {
		return err
	}
	log.Infow("doJob2", "sz", sz)
	return nil
}

func doJob3(ctx context.Context, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {

	//465

	cluster := gocql.NewCluster("10.0.3.47")
	cluster.Timeout = 60 * time.Second
	cluster.Keyspace = "idx"

	session, err := cluster.CreateSession()
	if err != nil {
		return err
	}

	payloadHashs := make([][]byte, 0, 70000)
	var payloadHash []byte
	payloadHashQry := `select payloadmultihash from txpieceblockoffsetsizev1`
	payloadHashQryIter := session.Query(payloadHashQry).WithContext(ctx).Iter()
	for payloadHashQryIter.Scan(&payloadHash) {
		if payloadHash == nil {
			log.Infow("payloadHash == nil")
			continue
		}
		payloadHashs = append(payloadHashs, bytes.Clone(payloadHash))
		//payloadHashStr := hex.EncodeToString(payloadHash)
		//log.Infow("dojob3", "pieceHexStr", pieceHexStr, "payloadHashStr", payloadHashStr)
	}
	log.Infow("dojob3", "payloadHashs", len(payloadHashs))

	{
		pHex, err := hex.DecodeString("0181e2039220200d155e760dd8e7551c0e61b99db6226e6b6c94a90e71e1dc1e845757cd51e836")
		if err != nil {
			return err
		}
		hs := make([][]byte, 0, 2000)
		qry := `select payloadmultihash from payloadtopieces where piececid=?`
		iter1 := session.Query(qry, pHex).WithContext(ctx).Iter()
		var h []byte
		for iter1.Scan(&h) {
			hs = append(hs, bytes.Clone(h))
		}
		log.Infow("dojob3", "hs", len(hs))

		for _, item := range hs {
			for _, ph := range payloadHashs {
				a1 := trimMultihash(ph)
				if bytes.Equal(a1, item) {
					log.Infow("dojob3.e")
				}
			}

		}
	}

	txPieces := make([]cid.Cid, 0, 2000)
	qry := `select piececid from txcarpieces`
	iter := session.Query(qry).WithContext(ctx).Iter()

	var pieceStr string
	for iter.Scan(&pieceStr) {
		pieceCid, err := cid.Decode(pieceStr)
		if err != nil {
			return nil
		}
		txPieces = append(txPieces, pieceCid)
	}

	const InsertConcurrency = 4
	const InsertBatchSize = 10_000
	threadBatch := len(payloadHashs) / InsertConcurrency
	if threadBatch == 0 {
		threadBatch = len(payloadHashs)
	}

	deletePieceOffsetsQry1 := `delete from payloadtopieces where payloadmultihash=? and piececid=?`
	for pi, pieceCid := range txPieces {

		pieceHexStr := hex.EncodeToString(pieceCid.Bytes())
		log.Infow("dojob3", "pieceHexStr", pieceHexStr)
		pieceCidBytes := pieceCid.Bytes()

		var eg errgroup.Group
		for i := 0; i < len(payloadHashs); i += threadBatch {
			i := i
			j := i + threadBatch
			if j >= len(payloadHashs) {
				j = len(payloadHashs)
			}

			// Process batch recs[i:j]

			eg.Go(func() error {
				var batch *gocql.Batch
				recsb := payloadHashs[i:j]
				for allIdx, rec := range recsb {
					if batch == nil {
						batch = session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)
						batch.Entries = make([]gocql.BatchEntry, 0, InsertBatchSize)
					}

					batch.Entries = append(batch.Entries, gocql.BatchEntry{
						Stmt:       deletePieceOffsetsQry1,
						Args:       []interface{}{trimMultihash(rec), pieceCidBytes},
						Idempotent: true,
					})

					if allIdx == len(recsb)-1 || len(batch.Entries) == InsertBatchSize {
						err := func() error {
							defer func(start time.Time) {
								log.Infow("addMultihashesToPieces executeBatch", "i", i, "j", j, "took", time.Since(start), "entries", len(batch.Entries))
							}(time.Now())
							err := session.ExecuteBatch(batch)
							if err != nil {
								return fmt.Errorf("inserting into PayloadToPieces: %w", err)
							}
							return nil
						}()
						if err != nil {
							return err
						}
						batch = nil
					}
				}
				return nil
			})
		}

		err = eg.Wait()
		if err != nil {
			return err
		}
		log.Infow("c1", "pi", pi)
		//for hi, ph := range payloadHashs {
		//	log.Infow("process delete - 1", "pi", pi, "hi", hi)
		//
		//	delQry1 := `delete from payloadtopieces where payloadmultihash=? and piececid=?`
		//	err = session.Query(delQry1, trimMultihash(ph), pieceCid.Bytes()).WithContext(ctx).Exec()
		//	if err != nil {
		//		return err
		//	}
		//
		//	delQry2 := `delete from PieceBlockOffsetSize where PayloadMultihash=? and PieceCid=?`
		//	err = session.Query(delQry2, ph, pieceCid.Bytes()).WithContext(ctx).Exec()
		//	if err != nil {
		//		return err
		//	}
		//}
	}

	deletePieceOffsetsQry2 := `delete from PieceBlockOffsetSize where PayloadMultihash=? and PieceCid=?`
	for pi, pieceCid := range txPieces {

		pieceHexStr := hex.EncodeToString(pieceCid.Bytes())
		log.Infow("dojob3", "pieceHexStr", pieceHexStr)
		pieceCidBytes := pieceCid.Bytes()

		var eg errgroup.Group
		for i := 0; i < len(payloadHashs); i += threadBatch {
			i := i
			j := i + threadBatch
			if j >= len(payloadHashs) {
				j = len(payloadHashs)
			}

			// Process batch recs[i:j]

			eg.Go(func() error {
				var batch *gocql.Batch
				recsb := payloadHashs[i:j]
				for allIdx, rec := range recsb {
					if batch == nil {
						batch = session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)
						batch.Entries = make([]gocql.BatchEntry, 0, InsertBatchSize)
					}

					batch.Entries = append(batch.Entries, gocql.BatchEntry{
						Stmt:       deletePieceOffsetsQry2,
						Args:       []interface{}{rec, pieceCidBytes},
						Idempotent: true,
					})

					if allIdx == len(recsb)-1 || len(batch.Entries) == InsertBatchSize {
						err := func() error {
							defer func(start time.Time) {
								log.Debugw("addMultihashesToPieces executeBatch", "took", time.Since(start), "entries", len(batch.Entries))
							}(time.Now())
							err := session.ExecuteBatch(batch)
							if err != nil {
								return fmt.Errorf("inserting into PayloadToPieces: %w", err)
							}
							return nil
						}()
						if err != nil {
							return err
						}
						batch = nil

						// emit progress only from batch 0
						if i == 0 {
							numberOfGoroutines := len(payloadHashs)/threadBatch + 1
							log.Infow("progress", "p", float64(numberOfGoroutines)*float64(allIdx+1)/float64(len(payloadHashs)))
						}
					}
				}
				return nil
			})
		}

		err = eg.Wait()
		log.Infow("c2", "pi", pi)
	}

	return nil
}

// Probability of a collision in two 24 byte hashes (birthday problem):
// 2^(24*8/2) = 8 x 10^28
const multihashLimitBytes = 24

// trimMultihash trims the multihash to the last multihashLimitBytes bytes
func trimMultihash(mh multihash.Multihash) []byte {
	var idx int
	if len(mh) > multihashLimitBytes {
		idx = len(mh) - multihashLimitBytes
	}
	return mh[idx:]
}

type SizeInfo struct {
	OnDisk int64
}

func FileSize(path string) (SizeInfo, error) {
	start := time.Now()

	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return xerrors.New("FileInfo.Sys of wrong type")
			}

			// NOTE: stat.Blocks is in 512B blocks, NOT in stat.Blksize		return SizeInfo{size}, nil
			//  See https://www.gnu.org/software/libc/manual/html_node/Attribute-Meanings.html
			size += int64(stat.Blocks) * 512 // nolint NOTE: int64 cast is needed on osx
		}
		return err
	})

	log.Infow("file size check", "took", time.Now().Sub(start), "path", path)
	if time.Now().Sub(start) >= 3*time.Second {
		log.Warnw("very slow file size check", "took", time.Now().Sub(start), "path", path)
	}

	if err != nil {
		if os.IsNotExist(err) {
			return SizeInfo{}, os.ErrNotExist
		}
		return SizeInfo{}, xerrors.Errorf("filepath.Walk err: %w", err)
	}

	return SizeInfo{size}, nil
}

func doJob4(ctx context.Context, db *db.DirectDealsDB, pd *piecedirectory.PieceDirectory) error {

	path := "/fc/cuseal"
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}

	capacity := int64(stat.Blocks) * int64(stat.Bsize)
	available := int64(stat.Bavail) * int64(stat.Bsize)
	fSAvailable := int64(stat.Bavail) * int64(stat.Bsize)

	log.Infow("doJob4", "capacity", capacity, "available", available, "fSAvailable", fSAvailable)
	return nil
}
