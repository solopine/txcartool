package node

import (
	"context"
	"database/sql"
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
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-metrics-interface"
	"github.com/solopine/txcartool/lib/boost/build"
	"github.com/solopine/txcartool/lib/boost/cmd/lib"
	"github.com/solopine/txcartool/lib/boost/db"
	"github.com/solopine/txcartool/lib/boost/node/config"
	"github.com/solopine/txcartool/lib/boost/node/modules"
	"github.com/solopine/txcartool/lib/boost/node/repo"
	"github.com/solopine/txcartool/lib/boost/piecedirectory"
	bdclient "github.com/solopine/txcartool/lib/boostd-data/client"
	"github.com/solopine/txcartool/lib/boostd-data/model"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
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
				return doJob(pdctx, db, pd)
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

	if err := pd.TxAddDealForPiece(ctx, entry.PieceCID, model.DealInfo{
		DealUuid:     entry.ID.String(),
		ChainDealID:  abi.DealID(entry.AllocationID), // Convert the type to avoid migration as underlying types are same
		MinerAddr:    entry.Provider,
		SectorID:     entry.SectorID,
		PieceOffset:  entry.Offset,
		PieceLength:  entry.Length,
		CarLength:    uint64(entry.InboundFileSize),
		IsDirectDeal: true,
	}); err != nil {
		return err
	}

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
