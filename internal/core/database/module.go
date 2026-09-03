package database

import (
	"context"
	"database/sql"

	"go.uber.org/fx"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/isolation"
	"librevita.org/internal/database/record"
	"librevita.org/pkg/log"
)

// Module provides the main store, raw database handle, and Ent ORM client,
// and manages their lifecycle.
var Module = fx.Module("database",
	fx.Provide(NewStore),
	fx.Provide(sqlDB),
	fx.Provide(entClient),
	fx.Invoke(registerLifecycle),
)

// sqlDB exposes the database handle: the embedded SQLite backend, PostgreSQL, or
// the dqlite wire protocol driver, both database/sql, so every consumer works unchanged.
func sqlDB(store *Store) *sql.DB { return store.SQL() }

// entClient exposes the Ent ORM client configured for the active persistence backend
// with compile-time typed blind indexing hooks and context-aware decryption interceptors.
func entClient(store *Store, hasher crypto.Hasher, encryptor crypto.Encryptor, engine *crypto.Engine) *record.Client {
	client := store.Ent()
	client.Use(isolation.MutationHook())
	client.Use(record.FLEMutationHook(hasher, encryptor, engine))
	client.Intercept(isolation.QueryInterceptor())
	client.Intercept(record.FLEDecryptionInterceptor(encryptor, engine))
	return client
}

func registerLifecycle(lc fx.Lifecycle, store *Store, logger log.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Apply the embedded migrations on boot for the active backend.
			if db := store.SQL(); db != nil {
				logger.Info("applying embedded Goose migrations",
					log.String("driver", store.Driver()),
				)
				if err := MigrateWithDriver(ctx, db, store.Driver(), logger); err != nil {
					return err
				}
				logger.Info("Goose migrations applied",
					log.String("driver", store.Driver()),
				)
			}
			if client := store.Ent(); client != nil {
				if err := SeedInitialData(clinicctx.WithSkipIsolation(ctx), client); err != nil {
					return err
				}
			}
			return nil
		},
		OnStop: func(context.Context) error {
			return store.Close()
		},
	})
}
