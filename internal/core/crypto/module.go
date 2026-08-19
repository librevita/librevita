package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module provides field-level encryption, envelope encryption,
// and blind indexing under the configured master key and injected KeyVault.
var Module = fx.Module("crypto",
	fx.Provide(
		NewFromConfig,
	),
)

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
