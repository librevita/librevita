package fhir

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/server"
	"librevita.org/pkg/log"
	authmocks "librevita.org/tests/mocks/core/auth"
)

func TestCSRFSkipperExemptsFHIRPrefix(t *testing.T) {
	csrf := auth.NewCSRF(&config.Config{Mode: "test"})
	e := echo.New()
	e.Use(server.CSRFMiddleware(csrf, csrfSkipper()))
	e.POST("/fhir/r4/Bundle", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.POST("/patients", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, ContentType)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("FHIR POST without CSRF status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/patients", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-FHIR POST without CSRF status = %d, want 403", rec.Code)
	}
}

func TestBodyLimitSkipperMatchesBundleRoute(t *testing.T) {
	e := echo.New()
	e.POST("/fhir/r4/Bundle", func(c echo.Context) error {
		if !bodyLimitSkipper()(c) {
			t.Fatal("expected body-limit skipper to match Bundle route")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRegisterHTTPRoutes(t *testing.T) {
	e := echo.New()
	sessRepo := authmocks.NewMockSessionRepository(t)
	logger := log.Nop()
	sessions, err := auth.NewSessionManager(sessRepo, &config.Config{Mode: "development"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	gate := func() echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}
	}
	h := &Handler{}
	registerHTTPRoutes(e, h, sessions, gate, logger)

	if len(e.Routes()) == 0 {
		t.Fatal("expected routes to be registered")
	}
	if Module == nil {
		t.Fatal("expected module to be defined")
	}
}

