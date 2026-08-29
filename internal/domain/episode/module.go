package episode

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/episode/delivery/http"
	episoderepository "librevita.org/internal/domain/episode/repository"
	"librevita.org/internal/domain/episode/usecase"
)

// Module wires the SOAP chart domain (repository, use case, HTML).
// FHIR communication is a separate, replaceable interop module.
var Module = fx.Module("episode",
	fx.Provide(episoderepository.NewEpisodeRepository),
	fx.Provide(usecase.NewService),
	fx.Provide(http.NewHandler),
	fx.Invoke(registerHTTPRoutes),
)

func registerHTTPRoutes(
	e *echo.Echo,
	h *http.Handler,
	sessions *auth.SessionManager,
	policies *policy.PolicyEngine,
	auditLogger *audit.Logger,
	gate server.SetupGate,
	log *slog.Logger,
) {
	setupGate := gate()
	view := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "chart.view"),
	}
	write := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "chart.write"),
	}

	e.GET("/patients/:id/episodes", h.List, view...)
	e.GET("/patients/:id/episodes/new", h.NewPage, write...)
	e.POST("/patients/:id/episodes", h.Create, write...)
	e.GET("/patients/:id/episodes/:episodeID", h.View, view...)
	e.GET("/patients/:id/episodes/:episodeID/edit", h.EditPage, write...)
	e.POST("/patients/:id/episodes/:episodeID", h.Update, write...)
	e.POST("/patients/:id/episodes/:episodeID/finalize", h.Finalize, write...)
	e.POST("/patients/:id/episodes/:episodeID/amend", h.Amend, write...)
}
