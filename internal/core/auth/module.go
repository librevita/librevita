package auth

import (
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module provides session management and CSRF protection.
var Module = fx.Module("auth",
	fx.Provide(NewSessionManager),
	fx.Provide(NewCSRF),
	fx.Invoke(configureConcurrency),
)

func configureConcurrency(cfg *config.Config) {
	SetMaxConcurrentHashes(cfg.Auth.MaxConcurrentHashes)
}
