package patient

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/normalize"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/patient/delivery/http"
	patientrepository "librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/log"
)

// Module wires the patient domain: usecase service, repository,
// and HTTP endpoints.
var Module = fx.Module("patient",
	fx.Provide(patientrepository.NewPatientRepositoryWithEngine),
	fx.Provide(usecase.NewService),
	fx.Provide(http.NewHandler),
	fx.Provide(
		fx.Annotate(
			bodyLimitSkipper,
			fx.ResultTags(`group:"bodyLimitSkip"`),
		),
	),
	fx.Invoke(registerHTTPRoutes),
	fx.Invoke(initSearchEngine),
)

func initSearchEngine(cfg *config.Config, logger log.Logger) error {
	if cfg == nil || cfg.Search.RulesFile == "" {
		return nil
	}
	engine, err := normalize.LoadRuleEngineFromFile(cfg.Search.RulesFile)
	if err != nil {
		return errors.Wrapf(err, "patient: load search rules from %s", cfg.Search.RulesFile)
	}
	normalize.SetActiveRuleEngine(engine)
	logger.Info("custom search phonetic rules loaded", log.String("path", cfg.Search.RulesFile))
	return nil
}

func bodyLimitSkipper() middleware.Skipper {
	return func(c echo.Context) bool {
		return c.Path() == "/patients/:id/documents"
	}
}

func registerHTTPRoutes(
	e *echo.Echo,
	h *http.Handler,
	sessions *auth.SessionManager,
	policies *policy.PolicyEngine,
	auditLogger *audit.Logger,
	gate server.SetupGate,
	logger log.Logger,
) {
	setupGate := gate()
	view := []echo.MiddlewareFunc{
		setupGate,
		server.RequireAuth(sessions, logger),
		server.RequirePolicy(policies, auditLogger, logger, "patient.view"),
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
	e.POST("/patients/:id/shred", h.Shred, setupGate, server.RequireAuth(sessions, logger), server.RequirePolicy(policies, auditLogger, logger, "patient.erase"))
	e.POST("/patients/bulk-archive", h.BulkArchive, view...)

	// Clinical attachments
	e.POST("/patients/:id/documents", h.UploadDocument,
		append(view, middleware.BodyLimit("25M"))...)
	e.GET("/patients/:id/documents/:docID", h.DownloadDocument, view...)
}
