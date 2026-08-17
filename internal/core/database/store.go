package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"librevita.org/internal/core/config"
)

// Store is the persistence handle produced by the factory.
//
// SQLite is the default backend. Set LIBREVITA_DB_DRIVER=dqlite to use
// a dqlite cluster through the pure-Go wire protocol driver.
type Store struct {
	driver string
	db     *sql.DB
}

// NewStore is the Fx provider for the configured backend.
func NewStore(cfg *config.Config, log *slog.Logger) (*Store, error) {
	switch cfg.Database.Driver {
	case config.DriverDqlite:
		db, err := openDqlite(cfg.Database.DqliteAddrs, cfg.Database.DqliteDiscoverySRV, cfg.Database.DqliteDatabase)
		if err != nil {
			return nil, err
		}
		log.Info("using dqlite persistence", "addrs", cfg.Database.DqliteAddrs, "discovery_srv", cfg.Database.DqliteDiscoverySRV, "database", cfg.Database.DqliteDatabase)
		return &Store{driver: config.DriverDqlite, db: db}, nil

	case config.DriverSQLite:
		db, err := openSQLite(cfg.Database.Path)
		if err != nil {
			return nil, err
		}
		log.Info("using SQLite/WAL persistence", "path", cfg.Database.Path)
		return &Store{driver: config.DriverSQLite, db: db}, nil

	default:
		// config.New validates this value; keep a defensive check here.
		return nil, fmt.Errorf("database: unknown driver %q", cfg.Database.Driver)
	}
}

// Driver returns the active backend ("sqlite" or "dqlite").
func (s *Store) Driver() string { return s.driver }

// SQL returns the database handle: the embedded SQLite backend or the
// dqlite wire protocol driver, both database/sql, so every consumer
// works unchanged.
func (s *Store) SQL() *sql.DB { return s.db }

// Close releases resources for the active backend.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
