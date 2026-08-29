package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

// Module manages the HTTP server lifecycle through Fx.
var Module = fx.Module("server",
	fx.Provide(provideEcho),
	fx.Invoke(registerLifecycle, registerNotFound),
)

type middlewareSkippers struct {
	fx.In
	CSRF      []middleware.Skipper `group:"csrfSkip"`
	BodyLimit []middleware.Skipper `group:"bodyLimitSkip"`
}

func provideEcho(csrf *auth.CSRF, cfg *config.Config, log *slog.Logger, skip middlewareSkippers) *echo.Echo {
	return newEcho(csrf, cfg, log, skip.CSRF, skip.BodyLimit)
}

// registerLifecycle starts Echo asynchronously and shuts it down gracefully.
func registerLifecycle(lc fx.Lifecycle, e *echo.Echo, cfg *config.Config, log *slog.Logger, shutdown fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("HTTP server listening", "addr", net.JoinHostPort(cfg.HTTPBind, strconv.Itoa(cfg.HTTPPort)))
				httpAddr := net.JoinHostPort(cfg.HTTPBind, strconv.Itoa(cfg.HTTPPort))
				if err := e.Start(httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
