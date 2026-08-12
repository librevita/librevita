package patient

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/patient/delivery/http"
	"librevita.org/internal/domain/patient/identifier"
	"librevita.org/internal/domain/patient/usecase"
)

// lookupLimit bounds the exact-document search. The blind index is an
// oracle over low-entropy values (a CPF space is ~10^9), so an
// authenticated caller must not be able to enumerate it quickly.
const lookupLimit = 60

// Module provides the patient registry: service, handlers, and routes,
// plus the identifier subsystem (FHIR-style systems and encrypted
// patient documents).
var Module = fx.Module("patient",
	fx.Provide(usecase.NewService),
	fx.Provide(identifier.NewRegistry),
	fx.Provide(identifier.NewSystemsService),
	fx.Provide(identifier.NewService),
	fx.Invoke(registerIdentifierLoader),
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

// registerIdentifierLoader fills the registry during OnStart, after the
// database module has applied the embedded migrations (fx.Invoke would
// run too early, against a schema that does not exist yet). A broken
// system configuration fails the boot with a clear message instead of
// silently degrading every document to the raw fallback.
func registerIdentifierLoader(lc fx.Lifecycle, db *sql.DB, reg *identifier.Registry, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			rows, err := identifier.LoadActiveSystems(ctx, db)
			if err != nil {
				log.Error("identifier: load systems", "error", err)
				return err
			}
			if err := reg.Reload(rows); err != nil {
				log.Error("identifier: load systems", "error", err)
				return err
			}
			return nil
		},
	})
}

func registerRoutes(e *echo.Echo, h *httphandler.Handler, gate server.SetupGate,
	sessions *auth.SessionManager, policies *policy.PolicyEngine,
	auditLogger *audit.Logger, log *slog.Logger) {

	view := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
	}
	// patient.edit is enforced as the fine-grained resource policy inside
	// the use cases, where the record attributes are available; the route
	// middleware only applies the coarse view filter.
	// The exact-document lookup is registered before the parametric
	// /patients/:id route, which Echo would otherwise shadow it with.
	e.GET("/patients/lookup", h.IdentifierLookup, append(view, server.RateLimit(server.NewRateLimiter(lookupLimit, time.Minute)))...)
	e.GET("/patients", h.List, view...)
	e.GET("/patients/new", h.NewPage, view...)
	e.POST("/patients", h.Create, view...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients/:id/edit", h.EditPage, view...)
	e.POST("/patients/:id", h.Update, view...)
	e.POST("/patients/:id/archive", h.Archive, view...)
	e.POST("/patients/:id/restore", h.Restore, view...)
	e.POST("/patients/bulk-archive", h.BulkArchive, view...)
	e.POST("/patients/:id/identifiers", h.IdentifierAdd, view...)
	e.POST("/patients/:id/identifiers/:identifierID/remove", h.IdentifierRemove, view...)

	// Administrator catalog of document systems. There is no delete:
	// patient identifiers reference the system URN, so systems are
	// deactivated instead. admin.view is the coarse gate; the catalog
	// lives in the patient domain, where the systems service is.
	admin := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "admin.view"),
	}
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)

	// Clinical attachments. Belonging to the patient is enforced per
	// request (domain + resource), so a bare file id never resolves an
	// attachment of another patient (IDOR). The upload route raises the
	// global 1 MiB body limit to the document cap.
	read := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.read"),
	}
	write := []echo.MiddlewareFunc{
		gate(), server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.write"),
		middleware.BodyLimit("25M"),
	}
	e.POST("/patients/:id/documents", h.UploadDocument, write...)
	e.GET("/patients/:id/documents/:fileID", h.DownloadDocument, read...)
}
