package yugabyte

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/yugabyte/gocql"
)

func (s *Store) CreateKeyspace(ctx context.Context) error {
	// Create a new session using the default keyspace, then use that to create
	// the new keyspace
	log.Infow("creating cassandra keyspace " + s.cluster.Keyspace)
	cluster := gocql.NewCluster(s.settings.Hosts...)
	cluster.Timeout = time.Duration(s.settings.CQLTimeout) * time.Second
	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("creating yugabyte cluster: %w", err)
	}
	query := `CREATE KEYSPACE IF NOT EXISTS ` + s.cluster.Keyspace +
		` WITH REPLICATION = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 }`
	log.Debug(query)
	return session.Query(query).WithContext(ctx).Exec()
}

func (s *Store) execScript(ctx context.Context, cqlstr string, exec func(context.Context, string) error) error {
	lines := strings.Split(cqlstr, ";")
	for _, line := range lines {
		line = strings.Trim(line, "\n \t")
		if line == "" {
			continue
		}
		log.Debug(line)
		err := exec(ctx, line)
		if err != nil {
			return fmt.Errorf("executing\n%s\n%w", line, err)
		}
	}

	return nil
}

func (s *Store) execCQL(ctx context.Context, query string) error {
	return s.session.Query(query).WithContext(ctx).Exec()
}

func (s *Store) execSQL(ctx context.Context, query string) error {
	_, err := s.db.Exec(ctx, query)
	return err
}
