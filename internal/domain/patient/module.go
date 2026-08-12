package patient

import (
	"context"
	"database/sql"
	"log/slog"

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

// Module provides the patient registry: service, handlers, and routes,
// plus the identifier subsystem (FHIR-style systems and encrypted
// patient documents).
var Module = fx.Module("patient",
	fx.Provide(usecase.NewService),
	fx.Provide(identifier.NewRegistry),
	fx.Provide(identifier.NewSystemsService),
	fx.Provide(identifier.NewService),
	fx.Invoke(loadIdentifierSystems),
	fx.Provide(httphandler.NewHandler),
	fx.Invoke(registerRoutes),
)

// loadIdentifierSystems fills the registry at boot. The seeded systems
// (or whatever an operator configured) are then immediately usable.
func loadIdentifierSystems(db *sql.DB, reg *identifier.Registry, log *slog.Logger) error {
	rows, err := identifier.LoadActiveSystems(context.Background(), db)
	if err != nil {
		log.Error("identifier: load systems", "error", err)
		return err
	}
	return reg.Reload(rows)
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
	e.GET("/patients", h.List, view...)
	e.GET("/patients/new", h.NewPage, view...)
	e.POST("/patients", h.Create, view...)
	e.POST("/patients/check-document", h.CheckDocument, view...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients/:id/edit", h.EditPage, view...)
	e.POST("/patients/:id", h.Update, view...)
	e.POST("/patients/:id/archive", h.Archive, view...)
	e.POST("/patients/:id/restore", h.Restore, view...)
	e.POST("/patients/bulk-archive", h.BulkArchive, view...)

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
