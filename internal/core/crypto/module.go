package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module provides field-level encryption and blind indexing under the
// configured master key.
var Module = fx.Module("crypto",
	fx.Provide(NewFromConfig),
)

// NewFromConfig is the Fx provider. The master key comes from
// config.MasterKey (base64, 32 bytes). Every environment except the
// explicit "development" requires the key; in development an ephemeral
// key is generated, which makes previously encrypted values
// undecryptable after restart.
func NewFromConfig(cfg *config.Config, log *slog.Logger) (*MasterKey, error) {
	if cfg.MasterKey == "" {
		if !cfg.IsDevelopment() {
			return nil, errors.New("crypto: master key is required outside development (LIBREVITA_MASTER_KEY)")
		}
		log.Warn("no master key configured; using an ephemeral key (encrypted values reset on restart)")
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("crypto: ephemeral master key: %w", err)
		}
		return derive(raw), nil
	}
	return NewMasterKey(cfg.MasterKey)
}
