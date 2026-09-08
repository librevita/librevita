//go:build no_postgres || (sqlite && !postgres)

package database

import (
	"database/sql"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/errors"
)

// openPostgres is a stub returning an error when PostgreSQL support is excluded from the build.
func openPostgres(_ config.PostgresConfig) (*sql.DB, error) {
	return nil, errors.New("database: postgres driver is not included in this build (compile without -tags sqlite or without -tags no_postgres)")
}
