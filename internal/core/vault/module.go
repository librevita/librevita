package vault

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

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
// Configured via cfg.Vault.Backend ("bbolt").
func NewKeyVaultFromConfig(cfg *config.Config, lc fx.Lifecycle, log *slog.Logger) (crypto.KeyVault, error) {
	switch strings.ToLower(cfg.Vault.Backend) {
	case "bbolt", "":
		dbPath := cfg.Vault.BBolt.Path
		if dbPath == "" {
			dbPath = filepath.Join(cfg.DataDir, "keys.db")
		}
		log.Info("initializing bbolt key vault adapter", "path", dbPath)
		vault, err := NewBBoltVault(dbPath)
		if err != nil {
			return nil, fmt.Errorf("vault: init bbolt: %w", err)
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing bbolt key vault adapter")
				return vault.Close()
			},
		})

		return vault, nil
	default:
		return nil, fmt.Errorf("vault: unsupported backend %q (use \"bbolt\")", cfg.Vault.Backend)
	}
}
