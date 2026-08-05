package database

import (
	"context"
	"log/slog"

	"go.uber.org/fx"
)

// Module provides the main store and manages its lifecycle.
var Module = fx.Module("database",
	fx.Provide(NewStore),
	fx.Invoke(registerLifecycle),
)

func registerLifecycle(lc fx.Lifecycle, store *Store, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Apply embedded migrations only for the SQLite backend.
			// rqlite manages schema changes through the cluster.
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
