package meta

import (
	"context"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/log"
)

// Module provides the meta Repository.
var Module = fx.Module("meta",
	fx.Provide(NewRepositoryFromConfig),
)

// NewRepositoryFromConfig opens the meta kv.Store (vault is not allowed).
func NewRepositoryFromConfig(cfg *config.Config, lc fx.Lifecycle, logger log.Logger) (Repository, error) {
	kvCfg := cfg.Meta
	if kvCfg.BBolt.Path == "" {
		kvCfg.BBolt.Path = filepath.Join(cfg.DataDir, "meta.db")
	}
	logger.Info("initializing meta store",
		log.String("backend", kvCfg.Backend),
		log.String("bbolt_path", kvCfg.BBolt.Path),
	)
	store, err := kv.Open(kvCfg)
	if err != nil {
		return nil, errors.Wrap(err, "meta: open")
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing meta store")
			return store.Close()
		},
	})
	return NewRepository(store), nil
}
