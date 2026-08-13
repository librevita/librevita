package database

import (
	"context"
	"database/sql"
	"log/slog"

	"go.uber.org/fx"
)

// Module provides the main store and manages its lifecycle.
var Module = fx.Module("database",
	fx.Provide(NewStore),
	fx.Provide(sqlDB),
	fx.Invoke(registerLifecycle),
)

// sqlDB exposes the database handle: the embedded SQLite backend or
// the dqlite wire protocol driver, both database/sql, so every
// consumer works unchanged.
func sqlDB(store *Store) *sql.DB { return store.SQL() }

func registerLifecycle(lc fx.Lifecycle, store *Store, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Apply the embedded migrations on boot for both backends:
			// dqlite embeds SQLite, so the schema is the same.
			if db := store.SQL(); db != nil {
				log.Info("applying embedded Goose migrations")
				if err := Migrate(ctx, db, log); err != nil {
					return err
				}
				log.Info("Goose migrations applied")
			}
			return nil
		},
		OnStop: func(context.Context) error {
			return store.Close()
		},
	})
}
