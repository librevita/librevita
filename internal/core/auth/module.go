package auth

import (
	"context"
	"log/slog"
	"time"

	"go.uber.org/fx"
)

// Module provides session management, password hashing, and authentication services.
var Module = fx.Module("auth",
	fx.Provide(
		NewSessionRepository,
		NewSessionManager,
		NewCSRF,
	),
	fx.Invoke(registerSessionCleaner),
)

func registerSessionCleaner(lc fx.Lifecycle, sessions *SessionManager, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				ticker := time.NewTicker(time.Hour)
				defer ticker.Stop()
				for {
					if err := sessions.CleanupExpired(ctx); err != nil {
						log.Warn("auth: cleanup expired sessions", "error", err)
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
