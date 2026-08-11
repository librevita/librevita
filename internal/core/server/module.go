package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

// Module manages the HTTP server lifecycle through Fx.
var Module = fx.Module("server",
	fx.Provide(New),
	fx.Invoke(registerLifecycle, registerNotFound),
)

// registerLifecycle starts Echo asynchronously and shuts it down gracefully.
func registerLifecycle(lc fx.Lifecycle, e *echo.Echo, cfg *config.Config, log *slog.Logger, shutdown fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("HTTP server listening", "addr", cfg.HTTPAddr)
				if err := e.Start(cfg.HTTPAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error("HTTP server failed", "error", err)
					_ = shutdown.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("shutting down HTTP server")
			return e.Shutdown(ctx)
		},
	})
}

// registerNotFound sends unauthenticated navigation on unknown routes to
// the login page (remembering the destination), so every URL — valid or
// not — returns to where the user was going after signing in. Unknown
// routes remain 404 for authenticated users and non-GET methods.
func registerNotFound(e *echo.Echo, sessions *auth.SessionManager) {
	echo.NotFoundHandler = func(c echo.Context) error {
		req := c.Request()
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			return echo.ErrNotFound
		}
		path := req.URL.Path
		if path == LoginPath || path == "/setup" || strings.HasPrefix(path, "/static/") || path == "/healthz" {
			return echo.ErrNotFound
		}
		if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
			if _, err := sessions.Authenticate(req.Context(), cookie.Value); err == nil {
				return echo.ErrNotFound
			}
		}
		return redirectLogin(c)
	}
}
