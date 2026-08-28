package database

import (
	"database/sql"
	"log/slog"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cockroachdb/errors"

	"librevita.org/ent"
	"librevita.org/internal/core/config"
)

// Store is the persistence handle produced by the factory.
//
// SQLite is the default backend. Set LIBREVITA_DB_DRIVER=postgres for PostgreSQL
// or LIBREVITA_DB_DRIVER=dqlite for a dqlite cluster.
type Store struct {
	driver string
	db     *sql.DB
	ent    *ent.Client
}

// NewStore is the Fx provider for the configured backend.
func NewStore(cfg *config.Config, log *slog.Logger) (*Store, error) {
	switch cfg.Database.Driver {
	case config.DriverPostgres:
		db, err := openPostgres(cfg.Database.Postgres)
		if err != nil {
			return nil, err
		}
		drv := entsql.OpenDB(dialect.Postgres, db)
		entClient := ent.NewClient(ent.Driver(drv))
		log.Info("using PostgreSQL persistence", "host", cfg.Database.Postgres.Host, "database", cfg.Database.Postgres.Database)
		return &Store{driver: config.DriverPostgres, db: db, ent: entClient}, nil

	case config.DriverDqlite:
		db, err := openDqlite(cfg.Database.Dqlite.Addrs, cfg.Database.Dqlite.DiscoverySRV, cfg.Database.Dqlite.Database)
		if err != nil {
			return nil, err
		}
		drv := entsql.OpenDB(dialect.SQLite, db)
		entClient := ent.NewClient(ent.Driver(drv))
		log.Info("using dqlite persistence", "addrs", cfg.Database.Dqlite.Addrs, "discovery_srv", cfg.Database.Dqlite.DiscoverySRV, "database", cfg.Database.Dqlite.Database)
		return &Store{driver: config.DriverDqlite, db: db, ent: entClient}, nil

	case config.DriverSQLite:
		db, err := openSQLite(cfg.Database.SQLite.Path)
		if err != nil {
			return nil, err
		}
		drv := entsql.OpenDB(dialect.SQLite, db)
		entClient := ent.NewClient(ent.Driver(drv))
		log.Info("using SQLite/WAL persistence", "path", cfg.Database.SQLite.Path)
		return &Store{driver: config.DriverSQLite, db: db, ent: entClient}, nil

	default:
		// config.New validates this value; keep a defensive check here.
		return nil, errors.Newf("database: unknown driver %q", cfg.Database.Driver)
	}
}

// Driver returns the active backend ("sqlite", "postgres", or "dqlite").
func (s *Store) Driver() string { return s.driver }

// SQL returns the database handle: the embedded SQLite backend, PostgreSQL, or the
// dqlite wire protocol driver, both database/sql, so every consumer works unchanged.
func (s *Store) SQL() *sql.DB { return s.db }

// Ent returns the Ent ORM client configured for the active persistence backend.
func (s *Store) Ent() *ent.Client { return s.ent }

// Close releases resources for the active backend.
func (s *Store) Close() error {
	if s.ent != nil {
		_ = s.ent.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
