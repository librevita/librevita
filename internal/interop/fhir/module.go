package fhir

import (
	"log/slog"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/fx"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
)

// Module is the replaceable FHIR R4 communication facade. It depends on
// the SOAP episode use case and never lives inside the clinical domain.
var Module = fx.Module("fhir-r4",
	fx.Provide(
		NewHandler,
		fx.Annotate(
			csrfSkipper,
			fx.ResultTags(`group:"csrfSkip"`),
		),
		fx.Annotate(
			bodyLimitSkipper,
			fx.ResultTags(`group:"bodyLimitSkip"`),
		),
	),
	fx.Invoke(registerHTTPRoutes),
)

func csrfSkipper() middleware.Skipper {
	return func(c echo.Context) bool {
		return strings.HasPrefix(c.Request().URL.Path, "/fhir/r4")
	}
}

func bodyLimitSkipper() middleware.Skipper {
	return func(c echo.Context) bool {
		return c.Path() == "/fhir/r4/Bundle"
	}
}

func registerHTTPRoutes(
	e *echo.Echo,
	h *Handler,
	sessions *auth.SessionManager,
	gate server.SetupGate,
	log *slog.Logger,
) {
	authn := []echo.MiddlewareFunc{
		gate(),
		server.RequireAuth(sessions, log),
	}
	write := append(authn, middleware.BodyLimit("2M"))

	e.GET("/fhir/r4/metadata", h.Metadata, authn...)
	e.POST("/fhir/r4/Bundle", h.CreateBundle, write...)
	e.GET("/fhir/r4/Composition/:id/$document", h.Document, authn...)
	e.GET("/fhir/r4/Encounter/:id", h.GetEncounter, authn...)
	e.GET("/fhir/r4/Encounter", h.SearchEncounter, authn...)
}
