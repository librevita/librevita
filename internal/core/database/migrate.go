package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"

	dbassets "librevita.org/db"
)

// migrationsDir is the directory inside the embedded filesystem.
const migrationsDir = "migrations"

// Migrate applies all pending Goose migrations to SQLite.
// Migrations are embedded in the binary and are safe to run on every boot.

func Migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	provider, err := newProvider(db, log)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: goose up: %w", err)
	}
	return nil
}

func newProvider(db *sql.DB, log *slog.Logger) (*goose.Provider, error) {
	migrations, err := fs.Sub(dbassets.Migrations, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("migrate: embedded filesystem: %w", err)
	}
	return goose.NewProvider(
		goose.DialectSQLite3,
		db,
		migrations,
		goose.WithSlog(log),
		goose.WithVerbose(true),
	)
}
