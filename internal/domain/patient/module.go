package patient

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/patient/delivery/http"
	"librevita.org/internal/domain/patient/usecase"
)

// Module provides the patient registry: service, handlers, and routes.
var Module = fx.Module("patient",
	fx.Provide(usecase.NewService),
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, gate server.SetupGate,
	sessions *auth.SessionManager, policies *policy.PolicyEngine,
	auditLogger *audit.Logger, log *slog.Logger) {

	view := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
	}
	edit := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.edit"),
	}

	e.GET("/patients", h.List, view...)
	e.GET("/patients/new", h.NewPage, edit...)
	e.POST("/patients", h.Create, edit...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients/:id/edit", h.EditPage, edit...)
	e.POST("/patients/:id", h.Update, edit...)
	e.POST("/patients/:id/archive", h.Archive, edit...)
	e.POST("/patients/:id/restore", h.Restore, edit...)
}
