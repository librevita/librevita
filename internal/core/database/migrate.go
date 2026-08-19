package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"

	"librevita.org/internal/core/config"
	"librevita.org/internal/database/migrations"
)

// Migrate applies all pending Goose migrations to SQLite.
// Migrations are embedded in the binary and are safe to run on every boot.
func Migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	return MigrateWithDriver(ctx, db, config.DriverSQLite, log)
}

// MigrateWithDriver applies all pending Goose migrations for the specified database driver.
func MigrateWithDriver(ctx context.Context, db *sql.DB, driver string, log *slog.Logger) error {
	provider, err := newProvider(db, driver, log)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: goose up: %w", err)
	}
	return nil
}

func newProvider(db *sql.DB, driver string, log *slog.Logger) (*goose.Provider, error) {
	subPath := "sqlite"
	dialect := goose.DialectSQLite3

	if driver == config.DriverPostgres {
		subPath = "postgres"
		dialect = goose.DialectPostgres
	}

	subFS, err := fs.Sub(migrations.FS, subPath)
	if err != nil {
		return nil, fmt.Errorf("migrate: embedded filesystem for %q: %w", subPath, err)
	}

	return goose.NewProvider(
		dialect,
		db,
		subFS,
		goose.WithSlog(log),
		goose.WithVerbose(true),
	)
}
