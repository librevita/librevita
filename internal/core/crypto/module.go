package crypto

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module provides field-level encryption, envelope encryption, key vault storage,
// and blind indexing under the configured master key.
var Module = fx.Module("crypto",
	fx.Provide(
		NewKeyVaultFromConfig,
		NewFromConfig,
	),
)

// NewKeyVaultFromConfig provides the KeyVault implementation.
// Configured via cfg.Vault.Backend ("bbolt").
func NewKeyVaultFromConfig(cfg *config.Config, lc fx.Lifecycle, log *slog.Logger) (KeyVault, error) {
	switch strings.ToLower(cfg.Vault.Backend) {
	case "bbolt", "":
		dbPath := cfg.Vault.BBolt.Path
		if dbPath == "" {
			dbPath = filepath.Join(cfg.DataDir, "keys.db")
		}
		log.Info("initializing bbolt key vault", "path", dbPath)
		vault, err := NewBBoltVault(dbPath)
		if err != nil {
			return nil, fmt.Errorf("crypto: init bbolt key vault: %w", err)
		}

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				log.Info("closing crypto key vault")
				return vault.Close()
			},
		})

		return vault, nil
	default:
		return nil, fmt.Errorf("crypto: unsupported vault backend %q (use \"bbolt\")", cfg.Vault.Backend)
	}
}

// NewFromConfig is the Fx provider for Engine/MasterKey.
func NewFromConfig(cfg *config.Config, vault KeyVault, log *slog.Logger) (*Engine, error) {
	masterKey := cfg.MasterKey
	if masterKey == "" {
		if !cfg.IsDevelopment() {
			return nil, errors.New("crypto: master key is required outside development (LIBREVITA_MASTER_KEY)")
		}
		log.Warn("no master key configured; using an ephemeral key (encrypted values reset on restart)")
		raw := make([]byte, SizeDEK)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("crypto: ephemeral master key: %w", err)
		}
		return deriveEngine(raw, vault), nil
	}
	return NewEngine(masterKey, vault)
}
