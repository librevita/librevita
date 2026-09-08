//go:build no_sqlite || (postgres && !sqlite)

package database

import (
	"database/sql"

	"librevita.org/pkg/errors"
)

// openSQLite is a stub returning an error when SQLite support is excluded from the build.
func openSQLite(_ string) (*sql.DB, error) {
	return nil, errors.New("database: sqlite driver is not included in this build (compile without -tags postgres or without -tags no_sqlite)")
}
