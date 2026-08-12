package database

import (
	"fmt"

	"github.com/rqlite/gorqlite"
)

// openRqlite connects to an rqlite cluster.
//
// gorqlite is not a database/sql driver; it communicates with rqlite over HTTP.
// Goose migrations therefore apply only to the embedded SQLite backend, and
// consumers that require the *sql.DB handle (audit, auth, policy, storage,
// the sqlc repositories) reject nil themselves.
func openRqlite(addr string) (*gorqlite.Connection, error) {
	conn, err := gorqlite.Open(addr)
	if err != nil {
		return nil, fmt.Errorf("rqlite: failed to connect to %q: %w", addr, err)
	}
	return conn, nil
}
