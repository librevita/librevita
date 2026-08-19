package patient

import (
	"context"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/patient/delivery/http"
	"librevita.org/internal/domain/patient/identifier"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
)

// Module wires the patient domain: usecase service, identifier system,
// repositories, and HTTP endpoints.
var Module = fx.Module("patient",
	fx.Provide(repository.NewPatientRepository),
	fx.Provide(repository.NewSystemRepository),
	fx.Provide(repository.NewIdentifierRepository),
	fx.Provide(usecase.NewService),
	fx.Provide(identifier.NewRegistry),
	fx.Provide(provideIdentifierService),
	fx.Provide(identifier.NewSystemsService),
	fx.Provide(http.NewHandler),
	fx.Invoke(loadIdentifierSystems),
	fx.Invoke(registerHTTPRoutes),
)

func loadIdentifierSystems(lc fx.Lifecycle, reg *identifier.Registry, repo identifier.SystemRepository, log *slog.Logger) {
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

func provideIdentifierService(repo identifier.IdentifierRepository, key *crypto.MasterKey, reg *identifier.Registry, log *slog.Logger) *identifier.Service {
	return identifier.NewService(repo, key, reg, log)
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
	view := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
	}
	admin := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "admin.view"),
	}
	lookupLimiter := server.NewRateLimiter(60, time.Minute)
	lookup := append(view, server.RateLimit(lookupLimiter))

	// Patient records
	e.GET("/patients/lookup", h.IdentifierLookup, lookup...)
	e.POST("/patients/:id/identifiers", h.IdentifierAdd, view...)
	e.POST("/patients/:id/identifiers/:identifierID/remove", h.IdentifierRemove, view...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients", h.List, view...)
	e.GET("/patients/new", h.NewPage, view...)
	e.POST("/patients", h.Create, view...)
	e.GET("/patients/:id/edit", h.EditPage, view...)
	e.POST("/patients/:id", h.Update, view...)
	e.POST("/patients/:id/archive", h.Archive, view...)
	e.POST("/patients/:id/restore", h.Restore, view...)
	e.POST("/patients/bulk-archive", h.BulkArchive, view...)

	// Clinical attachments
	e.POST("/patients/:id/documents", h.UploadDocument,
		append(view, middleware.BodyLimit("25M"))...)
	e.GET("/patients/:id/documents/:docID", h.DownloadDocument, view...)

	// Identifier systems catalog
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)
}
