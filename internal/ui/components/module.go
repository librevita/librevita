package components

import (
	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/pkg/log"
)

// Module serves the reusable UI components that need an HTTP endpoint,
// rendered as htmx fragments: today only the datepicker popover. The
// routes are gated like the views that embed the widgets; the fragment
// itself is read-only and derives everything from the query
// parameters, so it never touches the database.
var Module = fx.Module("components",
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, gate server.SetupGate,
	sessions *auth.SessionManager, policies *policy.PolicyEngine,
	auditLogger *audit.Logger, logger log.Logger) {

	view := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, logger),
		// Coarse view gate: the datepicker is embedded in the patient
		// forms today. A dedicated policy is not worth the noise until
		// another domain embeds the widget.
		server.RequirePolicy(policies, auditLogger, logger, "patient.view"),
	}
	e.GET("/ui/datepicker", datepickerPanelHandler, view...)
}
