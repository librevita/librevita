package patient

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/patient/delivery/http"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
)

// Module wires the patient domain: usecase service, repository,
// and HTTP endpoints.
var Module = fx.Module("patient",
	fx.Provide(repository.NewPatientRepository),
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
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
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
}
