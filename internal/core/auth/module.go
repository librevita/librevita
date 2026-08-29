package auth

import (
	"context"
	"time"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
)

// Module provides session management, password hashing, and authentication services.
var Module = fx.Module("auth",
	fx.Provide(
		NewSessionRepository,
		NewPlatformSessionRepository,
		provideSessionManager,
		NewCSRF,
	),
	fx.Invoke(registerSessionCleaner),
)

func provideSessionManager(repo SessionRepository, platform PlatformSessionRepository, cfg *config.Config, logger log.Logger) (*SessionManager, error) {
	m, err := NewSessionManager(repo, cfg, logger)
	if err != nil {
		return nil, err
	}
	m.SetPlatformRepository(platform)
	return m, nil
}

func registerSessionCleaner(lc fx.Lifecycle, sessions *SessionManager, logger log.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				ticker := time.NewTicker(time.Hour)
				defer ticker.Stop()
				for {
					if err := sessions.CleanupExpired(ctx); err != nil {
						logger.Warn("auth: cleanup expired sessions", log.Error(err))
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						return
					}
				}
			}()
			return nil
		},
	})
}
