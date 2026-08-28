// Package database provides the LibreVita connection factory.
package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/cockroachdb/errors"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver (pure Go, CGO-free)

	"librevita.org/internal/core/config"
)

const pgxDriver = "pgx"

// openPostgres opens a PostgreSQL connection pool using pgx/stdlib (pure Go).
func openPostgres(cfg config.PostgresConfig) (*sql.DB, error) {
	dsn := cfg.DSN()
	db, err := sql.Open(pgxDriver, dsn)
	if err != nil {
		return nil, errors.Wrap(err, "postgres: failed to open connection")
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.Wrapf(err, "postgres: ping failed for host %q", cfg.Host)
	}

	return db, nil
}
