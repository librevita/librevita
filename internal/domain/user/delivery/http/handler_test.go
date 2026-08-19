package http_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/core/vault"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	patientrepo "librevita.org/internal/domain/patient/repository"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	userrepo "librevita.org/internal/domain/user/repository"
	"librevita.org/internal/domain/user/usecase"
)

func openHandlerDB(t *testing.T) (*sql.DB, *ent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:handler-test-"+uuid.NewString()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	return db, client
}

func newHandler(t *testing.T, client *ent.Client) (*httphandler.Handler, *auth.SessionManager, *usecase.Service) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(auth.NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(audit.NewAuditRepository(client), log)
	if err != nil {
		t.Fatal(err)
	}

	userRepo := userrepo.NewUserRepository(client)
	roleRepo := userrepo.NewRoleRepository(client)
	specialtyRepo := userrepo.NewSpecialtyRepository(client)
	staffReqRepo := userrepo.NewStaffRequestRepository(client)
	setupRepo := userrepo.NewSetupRepository(client)

	svc := usecase.NewService(userRepo, roleRepo, specialtyRepo, staffReqRepo, setupRepo, sessions, auditLogger, log)
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	policies, err := policy.NewPolicyEngine(policy.NewPolicyRepository(client), log)
	if err != nil {
		t.Fatal(err)
	}
	if err := policies.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	v, err := vault.NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })

	engine, err := crypto.NewEngine("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v)
	if err != nil {
		t.Fatal(err)
	}

	patientSvc := patientusecase.NewService(patientrepo.NewPatientRepository(client), engine, log, policies)
	files := mustFileManager(t, client)
	h := httphandler.NewHandler(svc, patientSvc, csrf, sessions, policies, auditLogger, clinicusecase.NewClockProvider(clinicrepo.NewClinicRepository(client)), files, slog.New(slog.DiscardHandler))
	return h, sessions, svc
}

func TestLogoutSurfacesRevocationFailure(t *testing.T) {
	db, client := openHandlerDB(t)
	h, sessions, _ := newHandler(t, client)

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

// mustFileManager builds a FileManager over a temp local store.
func mustFileManager(t *testing.T, client *ent.Client) *storage.FileManager {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fm, err := storage.NewFileManager(storage.NewIndexRepository(client), s, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return fm
}
