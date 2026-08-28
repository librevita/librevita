package vault

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
)

// Module provides the crypto.KeyVault implementation to Fx based on configuration.
var Module = fx.Module("vault",
	fx.Provide(
		NewKeyVaultFromConfig,
	),
)

// NewKeyVaultFromConfig provides the crypto.KeyVault implementation.
// Configured via cfg.Vault.Backend ("bbolt", "nats", "etcd", "hashicorp").
func NewKeyVaultFromConfig(cfg *config.Config, lc fx.Lifecycle, log *slog.Logger) (crypto.KeyVault, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.Vault.Backend))
	switch backend {
	case "bbolt", "":
		dbPath := cfg.Vault.BBolt.Path
		if dbPath == "" {
			dbPath = filepath.Join(cfg.DataDir, "keys.db")
		}
		log.Info("initializing bbolt key vault adapter", "path", dbPath)
		v, err := NewBBoltVault(dbPath)
		if err != nil {
			return nil, errors.Wrap(err, "vault: init bbolt")
		}
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing bbolt key vault adapter")
				return v.Close()
			},
		})
		return v, nil

	case "nats":
		url := cfg.Vault.NATS.URL
		bucket := cfg.Vault.NATS.Bucket
		log.Info("initializing nats jetstream key vault adapter", "url", url, "bucket", bucket)
		v, err := NewNATSVault(url, bucket)
		if err != nil {
			return nil, errors.Wrap(err, "vault: init nats")
		}
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing nats key vault adapter")
				return v.Close()
			},
		})
		return v, nil

	case "etcd":
		endpoints := cfg.Vault.Etcd.Endpoints
		prefix := cfg.Vault.Etcd.Prefix
		log.Info("initializing etcd v3 key vault adapter", "endpoints", endpoints, "prefix", prefix)
		v, err := NewEtcdVault(endpoints, prefix)
		if err != nil {
			return nil, errors.Wrap(err, "vault: init etcd")
		}
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing etcd key vault adapter")
				return v.Close()
			},
		})
		return v, nil

	case "hashicorp", "hashicorp_vault", "openbao":
		address := cfg.Vault.HashiCorp.Address
		token := cfg.Vault.HashiCorp.Token
		mount := cfg.Vault.HashiCorp.Mount
		log.Info("initializing hashicorp vault key vault adapter", "address", address, "mount", mount)
		v, err := NewHashiCorpVault(address, token, mount)
		if err != nil {
			return nil, errors.Wrap(err, "vault: init hashicorp vault")
		}
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing hashicorp key vault adapter")
				return v.Close()
			},
		})
		return v, nil

	default:
		return nil, errors.Newf("vault: unsupported backend %q (use \"bbolt\", \"nats\", \"etcd\", \"hashicorp\", \"hashicorp_vault\", or \"openbao\")", cfg.Vault.Backend)
	}
}
