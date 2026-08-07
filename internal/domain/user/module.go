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

// Per-client rate limits. Setup is the only way to become the initial
// admin, so it is deliberately strict.
const (
	loginLimit    = 10
	registerLimit = 5
	setupLimit    = 5
	limitWindow   = time.Minute
)

// Module provides the authentication service, handlers, and routes.
var Module = fx.Module("user",
	fx.Provide(usecase.NewService),
	fx.Provide(httphandler.NewHandler),
	// Share the setup gate with other modules so every route is gated
	// uniformly.
	fx.Provide(func(h *httphandler.Handler) server.SetupGate {
		return h.SetupGate
	}),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, log *slog.Logger) {

	loginLimiter := server.NewRateLimiter(loginLimit, limitWindow)
	registerLimiter := server.NewRateLimiter(registerLimit, limitWindow)
	setupLimiter := server.NewRateLimiter(setupLimit, limitWindow)
	gate := h.SetupGate()

	e.GET("/setup", h.SetupPage)
	e.POST("/setup", h.Setup, server.RateLimit(setupLimiter))
	e.GET("/auth/login", h.LoginPage, gate)
	e.POST("/auth/login", h.Login, gate, server.RateLimit(loginLimiter))
	// Registration is never public; the users.register policy decides who
	// may create accounts.
	e.GET("/auth/register", h.RegisterPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.register"))
	e.POST("/auth/register", h.Register, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.register"), server.RateLimit(registerLimiter))
	e.POST("/auth/logout", h.Logout, gate, server.RequireAuth(sessions, log))

	e.GET("/", h.Home, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/activity/recent", h.HomeActivity, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/profile", h.ProfilePage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/admin", h.Admin, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.GET("/admin/policies", h.AdminPoliciesPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.POST("/admin/policies", h.AdminPolicySave, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.POST("/admin/policies/reset", h.AdminPolicyReset, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
}
