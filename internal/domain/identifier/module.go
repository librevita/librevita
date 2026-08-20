package identifier

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/identifier/delivery/http"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/internal/domain/identifier/repository"
	"librevita.org/internal/domain/identifier/usecase"
)

// Module wires the identifier domain: repositories, registry, usecase services,
// HTTP delivery handlers, seed lifecycle hooks, and routes.
var Module = fx.Module("identifier",
	fx.Provide(repository.NewSystemRepository),
	fx.Provide(repository.NewIdentifierRepository),
	fx.Provide(identifiermodel.NewRegistry),
	fx.Provide(usecase.NewService),
	fx.Provide(usecase.NewSystemsService),
	fx.Provide(http.NewHandler),
	fx.Invoke(loadIdentifierSystems),
	fx.Invoke(registerHTTPRoutes),
)

func loadIdentifierSystems(lc fx.Lifecycle, reg *identifiermodel.Registry, repo identifiermodel.SystemRepository, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := repo.SeedDefaults(ctx); err != nil {
				log.Warn("failed to seed default identifier systems", "error", err)
			}
			rows, err := repo.ListActive(ctx)
			if err != nil {
				return err
			}
			if err := reg.Reload(rows); err != nil {
				log.Warn("failed to load some identifier systems from db", "error", err)
			}
			return nil
		},
	})
}

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
	admin := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "admin.view"),
	}

	// Identifier systems catalog
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)
}
