// Package server provides the Echo HTTP server managed by Fx. It hosts the
// transport adapters for authentication and authorization middleware;
// business logic stays in the auth and policy packages.
package server

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

type middlewareSkippers struct {
	fx.In
	CSRF      []middleware.Skipper `group:"csrfSkip"`
	BodyLimit []middleware.Skipper `group:"bodyLimitSkip"`
}

// New creates the Echo instance and installs global middleware.
func New(csrf *auth.CSRF, cfg *config.Config, log *slog.Logger, skip middlewareSkippers) *echo.Echo {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ProblemErrorHandler
	e.IPExtractor = clientIPExtractor(cfg.TrustedProxies)

	// Middleware order is significant. Clinic Host resolution is
	// registered with Echo.Pre from the clinic domain so this package
	// does not import domain repositories.
	e.Use(middleware.RequestID())
	// RequestLog sits outside Recover so recovered panics surface as 500s.
	e.Use(RequestLog(log))
	e.Use(middleware.Recover())
	// Same-origin application; no CORS is configured. Do not add a
	// permissive CORS middleware for future authenticated endpoints.
	e.Use(SecurityHeaders(cfg))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Limit: "1M",
		// File uploads raise the cap on their own routes and register a
		// skipper (Fx group bodyLimitSkip) so this package never lists
		// those paths.
		Skipper: func(c echo.Context) bool {
			return skipped(c, skip.BodyLimit)
		},
	}))
	e.Use(CSRFMiddleware(csrf, skip.CSRF...))

	// Read timeouts protect against slow-loris attacks. WriteTimeout stays
	// zero so that future Server-Sent Events are not interrupted.
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second

	e.GET(healthzPath, healthz)

	return e
}
