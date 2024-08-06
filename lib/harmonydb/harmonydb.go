package harmonydb

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/node/config"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lhdb "github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	logging "github.com/ipfs/go-log/v2"
	"github.com/yugabyte/pgx/v5"
	"github.com/yugabyte/pgx/v5/pgconn"
	"github.com/yugabyte/pgx/v5/pgxpool"
)

type ITestID string

// ItestNewID see ITestWithID doc
func ITestNewID() ITestID {
	return ITestID(strconv.Itoa(rand.Intn(99999)))
}

type DB struct {
	pgx       *pgxpool.Pool
	cfg       *pgxpool.Config
	schema    string
	hostnames []string
	BTFPOnce  sync.Once
	BTFP      atomic.Uintptr // BeginTransactionFramePointer
}

var logger = logging.Logger("harmonydb")

// NewFromConfig is a convenience function.
// In usage:
//
//	db, err := NewFromConfig(config.HarmonyDB)  // in binary init
func NewFromConfig(cfg config.HarmonyDB) (*DB, error) {
	return New(
		cfg.Hosts,
		cfg.Username,
		cfg.Password,
		cfg.Database,
		cfg.Port,
		"",
	)
}

// New is to be called once per binary to establish the pool.
// log() is for errors. It returns an upgraded database's connection.
// This entry point serves both production and integration tests, so it's more DI.
func New(hosts []string, username, password, database, port string, itestID ITestID) (*DB, error) {
	itest := string(itestID)
	connString := ""
	if len(hosts) > 0 {
		connString = "host=" + hosts[0] + " "
	}
	for k, v := range map[string]string{"user": username, "password": password, "dbname": database, "port": port} {
		if strings.TrimSpace(v) != "" {
			connString += k + "=" + v + " "
		}
	}

	schema := "curio"
	if itest != "" {
		schema = "itest_" + itest
	}

	cfg, err := pgxpool.ParseConfig(connString + "search_path=" + schema)
	if err != nil {
		return nil, err
	}

	// enable multiple fallback hosts.
	for _, h := range hosts[1:] {
		cfg.ConnConfig.Fallbacks = append(cfg.ConnConfig.Fallbacks, &pgconn.FallbackConfig{Host: h})
	}

	cfg.ConnConfig.OnNotice = func(conn *pgconn.PgConn, n *pgconn.Notice) {
		logger.Debug("database notice: " + n.Message + ": " + n.Detail)
		lhdb.DBMeasures.Errors.M(1)
	}

	db := DB{cfg: cfg, schema: schema, hostnames: hosts} // pgx populated in AddStatsAndConnect
	if err := db.addStatsAndConnect(); err != nil {
		return nil, err
	}

	return &db, nil
}

type tracer struct {
}

type ctxkey string

const SQL_START = ctxkey("sqlStart")
const SQL_STRING = ctxkey("sqlString")

func (t tracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(context.WithValue(ctx, SQL_START, time.Now()), SQL_STRING, data.SQL)
}
func (t tracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	lhdb.DBMeasures.Hits.M(1)
	ms := time.Since(ctx.Value(SQL_START).(time.Time)).Milliseconds()
	lhdb.DBMeasures.TotalWait.M(ms)
	lhdb.DBMeasures.Waits.Observe(float64(ms))
	if data.Err != nil {
		lhdb.DBMeasures.Errors.M(1)
	}
	logger.Debugw("SQL run",
		"query", ctx.Value(SQL_STRING).(string),
		"err", data.Err,
		"rowCt", data.CommandTag.RowsAffected(),
		"milliseconds", ms)
}

func (db *DB) GetRoutableIP() (string, error) {
	tx, err := db.pgx.Begin(context.Background())
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	local := tx.Conn().PgConn().Conn().LocalAddr()
	addr, ok := local.(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("could not get local addr from %v", addr)
	}
	return addr.IP.String(), nil
}

// addStatsAndConnect connects a prometheus logger. Be sure to run this before using the DB.
func (db *DB) addStatsAndConnect() error {

	db.cfg.ConnConfig.Tracer = tracer{}

	hostnameToIndex := map[string]float64{}
	for i, h := range db.hostnames {
		hostnameToIndex[h] = float64(i)
	}
	db.cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		s := db.pgx.Stat()
		lhdb.DBMeasures.OpenConnections.M(int64(s.TotalConns()))
		lhdb.DBMeasures.WhichHost.Observe(hostnameToIndex[c.Config().Host])

		//FUTURE place for any connection seasoning
		return nil
	}

	// Timeout the first connection so we know if the DB is down.
	ctx, ctxClose := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer ctxClose()
	var err error
	db.pgx, err = pgxpool.NewWithConfig(ctx, db.cfg)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to connect to database: %v\n", err))
		return err
	}
	return nil
}
