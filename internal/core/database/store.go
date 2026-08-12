package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/rqlite/gorqlite"

	"librevita.org/internal/core/config"
)

// Store is the persistence handle produced by the factory.
//
// SQLite is the default backend. Set LIBREVITA_DB_DRIVER=rqlite to use a
// cluster through gorqlite.
type Store struct {
	driver string
	db     *sql.DB              // Embedded SQLite backend.
	rq     *gorqlite.Connection // rqlite backend.
}

// NewStore is the Fx provider for the configured backend.
func NewStore(cfg *config.Config, log *slog.Logger) (*Store, error) {
	switch cfg.Database.Driver {
	case config.DriverRqlite:
		rq, err := openRqlite(cfg.Database.RqliteAddr)
		if err != nil {
			return nil, err
		}
		log.Info("using rqlite persistence", "addr", cfg.Database.RqliteAddr)
		return &Store{driver: config.DriverRqlite, rq: rq}, nil

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

// Driver returns the active backend ("sqlite" or "rqlite").
func (s *Store) Driver() string { return s.driver }

// SQL returns the embedded SQLite handle. It is nil in rqlite mode;
// consumers that require local storage must reject nil themselves.
func (s *Store) SQL() *sql.DB { return s.db }

// Rqlite returns the rqlite connection. It is nil in SQLite mode.
func (s *Store) Rqlite() *gorqlite.Connection { return s.rq }

// Close releases resources for the active backend.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	// gorqlite is a stateless HTTP client; there is no connection to close.
	return nil
}
