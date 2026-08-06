// Package user wires the authentication domain into the application.
package user

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	"librevita.org/internal/domain/user/usecase"
)

// Per-client rate limits. The register endpoint is deliberately stricter:
// a public registration is the only way to become the initial admin.
const (
	loginLimit    = 10
	registerLimit = 5
	limitWindow   = time.Minute
)

// Module provides the authentication service, handlers, and routes.
var Module = fx.Module("user",
	fx.Provide(usecase.NewService),
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, log *slog.Logger) {

	loginLimiter := server.NewRateLimiter(loginLimit, limitWindow)
	registerLimiter := server.NewRateLimiter(registerLimit, limitWindow)

	e.GET("/auth/login", h.LoginPage)
	e.POST("/auth/login", h.Login, server.RateLimit(loginLimiter))
	e.GET("/auth/register", h.RegisterPage)
	e.POST("/auth/register", h.Register, server.RateLimit(registerLimiter))
	e.POST("/auth/logout", h.Logout, server.RequireAuth(sessions, log))

	e.GET("/", h.Home, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/admin", h.Admin, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
}
