package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/pressly/goose/v3"

	"librevita.org/internal/core/config"
	"librevita.org/internal/database/migrations"
	"librevita.org/pkg/log"
)

// Migrate applies all pending Goose migrations to SQLite.
// Migrations are embedded in the binary and are safe to run on every boot.
func Migrate(ctx context.Context, db *sql.DB, logger log.Logger) error {
	return MigrateWithDriver(ctx, db, config.DriverSQLite, logger)
}

// MigrateWithDriver applies all pending Goose migrations for the specified database driver.
func MigrateWithDriver(ctx context.Context, db *sql.DB, driver string, logger log.Logger) error {
	provider, err := newProvider(db, driver, logger)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return errors.Wrap(err, "migrate: goose up")
	}
	if err := EnsureAuditTriggers(ctx, db, driver); err != nil {
		return errors.Wrap(err, "migrate: audit triggers")
	}
	return nil
}

func newProvider(db *sql.DB, driver string, logger log.Logger) (*goose.Provider, error) {
	subPath := "sqlite"
	dialect := goose.DialectSQLite3

	if driver == config.DriverPostgres {
		subPath = "postgres"
		dialect = goose.DialectPostgres
	}

	subFS, err := fs.Sub(migrations.FS, subPath)
	if err != nil {
		return nil, errors.Wrapf(err, "migrate: embedded filesystem for %q", subPath)
	}

	if logger == nil {
		logger = log.Nop()
	}

	return goose.NewProvider(
		dialect,
		db,
		subFS,
		goose.WithLogger(gooseLogger{log: logger}),
		goose.WithVerbose(logger.Enabled(log.Debug)),
	)
}

type gooseLogger struct {
	log log.Logger
}

func (g gooseLogger) Printf(format string, v ...any) {
	msg := strings.TrimSpace(fmt.Sprintf(format, v...))
	if msg == "" {
		return
	}
	g.log.Info(msg)
}

func (g gooseLogger) Fatalf(format string, v ...any) {
	g.log.Error(strings.TrimSpace(fmt.Sprintf(format, v...)))
}
