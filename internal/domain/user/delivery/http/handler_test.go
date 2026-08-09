package http_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/domain/clinic"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	"librevita.org/internal/domain/user/usecase"
)

func openHandlerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:handler-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newHandler(t *testing.T, db *sql.DB) (*httphandler.Handler, *auth.SessionManager, *usecase.Service) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, log)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(db, log)
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewService(db, sessions, auditLogger, log)
	csrf := auth.NewCSRF(&config.Config{Env: "development"})
	policies, err := policy.NewPolicyEngine(db, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := policies.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	patientSvc := patientusecase.NewService(db, log, policies)
	h := httphandler.NewHandler(svc, patientSvc, csrf, sessions, policies, auditLogger, clinic.NewClockProvider(db))
	return h, sessions, svc
}

func TestLogoutSurfacesRevocationFailure(t *testing.T) {
	db := openHandlerDB(t)
	h, sessions, _ := newHandler(t, db)

	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: "01990000-0000-7000-8000-000000000001", Email: "ana@example.org", Name: "Ana", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}

	db.Close() // Simulate a database outage during logout.

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(sessions.Cookie(token))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.Logout(c)
	if err == nil {
		t.Fatal("Logout must return an error when revocation fails")
	}
}
