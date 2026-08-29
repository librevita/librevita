package calendar

import (
	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/calendar/delivery/http"
	"librevita.org/pkg/log"
)

// Module provides the clinic calendar page. The schedule is a static mock
// for now: the month grid is computed from the server clock and the
// appointments are fixtures, so nothing is persisted. The repository,
// service and SSE live updates arrive with the appointments feature.
var Module = fx.Module("calendar",
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, gate server.SetupGate,
	sessions *auth.SessionManager, policies *policy.PolicyEngine,
	auditLogger *audit.Logger, logger log.Logger) {
	view := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, logger),
		server.RequirePolicy(policies, auditLogger, logger, "calendar.view"),
	}
	e.GET("/calendar", h.Page, view...)
}
