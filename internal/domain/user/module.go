// Package user wires the authentication domain into the application.
package user

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	"librevita.org/internal/domain/user/usecase"
)

// Module provides the authentication service, handlers, and routes.
var Module = fx.Module("user",
	fx.Provide(usecase.NewService),
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, sessions *auth.SessionManager, policies *policy.PolicyEngine, log *slog.Logger) {
	e.GET("/auth/login", h.LoginPage)
	e.POST("/auth/login", h.Login)
	e.GET("/auth/register", h.RegisterPage)
	e.POST("/auth/register", h.Register)
	e.POST("/auth/logout", h.Logout, server.RequireAuth(sessions, log))

	e.GET("/", h.Home, server.RequireAuth(sessions, log), server.RequirePolicy(policies, log, "dashboard.view"))
	e.GET("/admin", h.Admin, server.RequireAuth(sessions, log), server.RequirePolicy(policies, log, "admin.view"))
}
