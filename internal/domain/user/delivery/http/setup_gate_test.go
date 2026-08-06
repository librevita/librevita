package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/domain/user/usecase"
)

func TestSetupGateRedirectsWhenNotOnboarded(t *testing.T) {
	db := openHandlerDB(t)
	h, _, _ := newHandler(t, db)

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, h.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/setup" {
		t.Fatalf("first access = %d %q, want 302 /setup", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSetupGatePassesWhenOnboarded(t *testing.T) {
	db := openHandlerDB(t)
	h, _, svc := newHandler(t, db)

	if _, _, err := svc.Onboard(context.Background(),
		validRegisterInput(), validClinicInput()); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, h.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("onboarded access = %d, want 200", rec.Code)
	}
}

func TestSetupGateExemptsSetupPath(t *testing.T) {
	db := openHandlerDB(t)
	h, _, _ := newHandler(t, db)

	e := echo.New()
	e.GET("/setup", func(c echo.Context) error { return c.String(http.StatusOK, "setup") }, h.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /setup before onboarding = %d, want 200", rec.Code)
	}
}

func validRegisterInput() usecase.RegisterInput {
	return usecase.RegisterInput{Name: "Ana", Email: "ana@example.org", Password: "password-123"}
}

func validClinicInput() usecase.ClinicInput {
	return usecase.ClinicInput{Name: "Clinica"}
}
