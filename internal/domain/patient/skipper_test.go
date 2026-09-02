package patient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	patienthttp "librevita.org/internal/domain/patient/delivery/http"
	"librevita.org/pkg/log"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	policymocks "librevita.org/tests/mocks/core/policy"
)

func TestBodyLimitSkipperMatchesDocumentRoute(t *testing.T) {
	e := echo.New()
	e.POST("/patients/:id/documents", func(c echo.Context) error {
		if !bodyLimitSkipper()(c) {
			t.Fatal("expected body-limit skipper to match document upload route")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/patients/01990000-0000-7000-8000-0000000000aa/documents", nil)
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

	policyRepo := policymocks.NewMockRepository(t)
	policies, err := policy.NewPolicyEngine(policyRepo, logger)
	if err != nil {
		t.Fatal(err)
	}

	auditRepo := auditmocks.NewMockRepository(t)
	auditLogger, err := audit.NewLogger(auditRepo, logger)
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

	h := &patienthttp.Handler{}
	registerHTTPRoutes(e, h, sessions, policies, auditLogger, gate, logger)

	if len(e.Routes()) == 0 {
		t.Fatal("expected routes to be registered")
	}
	if Module == nil {
		t.Fatal("expected module to be defined")
	}
}

