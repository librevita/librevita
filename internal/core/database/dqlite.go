package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/canonical/go-dqlite/v3/client"
	dqlitedriver "github.com/canonical/go-dqlite/v3/driver"
)

// nodeStore is an in-memory NodeStore seeded from the configuration:
// the driver probes these addresses to find the cluster leader.
type nodeStore struct{ addrs []string }

func (s *nodeStore) Get(context.Context) ([]client.NodeInfo, error) {
	out := make([]client.NodeInfo, 0, len(s.addrs))
	for _, a := range s.addrs {
		out = append(out, client.NodeInfo{Address: a})
	}
	return out, nil
}

func (s *nodeStore) Set(context.Context, []client.NodeInfo) error { return nil }

// openDqlite connects to the dqlite cluster through the pure-Go client
// driver (github.com/canonical/go-dqlite/v3). The driver performs real
// transactions (BEGIN/COMMIT via the wire protocol, replicated through
// Raft), prepared statements, and strong consistency. A single
// connection serializes writes, mirroring the SQLite backend.
func openDqlite(addrs string, database string) (*sql.DB, error) {
	addresses := make([]string, 0)
	for _, addr := range strings.Split(addrs, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			addresses = append(addresses, addr)
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("dqlite: no node addresses configured")
	}

	drv, err := dqlitedriver.New(&nodeStore{addrs: addresses})
	if err != nil {
		return nil, fmt.Errorf("dqlite: driver: %w", err)
	}
	connector, err := drv.OpenConnector(database)
	if err != nil {
		return nil, fmt.Errorf("dqlite: connector: %w", err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("dqlite: ping failed for %v: %w", addresses, err)
	}
	return db, nil
}
