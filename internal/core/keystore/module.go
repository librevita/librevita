package keystore

import (
	"context"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/log"
)

// Module provides crypto.KeyStore from the keystore config block.
var Module = fx.Module("keystore",
	fx.Provide(
		NewKeyStoreFromConfig,
	),
)

// NewKeyStoreFromConfig opens the keystore kv.Store (vault backend allowed)
// and wraps it with DEK shredding.
func NewKeyStoreFromConfig(cfg *config.Config, lc fx.Lifecycle, logger log.Logger) (crypto.KeyStore, error) {
	kvCfg := cfg.Keystore
	if kvCfg.BBolt.Path == "" {
		kvCfg.BBolt.Path = filepath.Join(cfg.DataDir, "keystore.db")
	}
	logger.Info("initializing keystore",
		log.String("backend", kvCfg.Backend),
		log.String("bbolt_path", kvCfg.BBolt.Path),
	)
	inner, err := kv.Open(kvCfg, kv.AllowVault())
	if err != nil {
		return nil, errors.Wrap(err, "keystore: open")
	}
	ks := Wrap(inner)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing keystore")
			return ks.Close()
		},
	})
	return ks, nil
}
