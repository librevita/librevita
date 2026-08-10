package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/types"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:server-test-"+uuid.NewString()+"?mode=memory&cache=shared")
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

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRequireAuthRedirectsAnonymous(t *testing.T) {
	db := openTestDB(t)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "ok") }, RequireAuth(sessions, testLogger()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != LoginPath {
		t.Fatalf("anonymous request = %d %q, want 302 %q", rec.Code, rec.Header().Get("Location"), LoginPath)
	}
}

func TestRequireAuthAcceptsValidSession(t *testing.T) {
	db := openTestDB(t)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role_id) VALUES (?, ?, ?, ?, ?)`,
		"01990000-0000-7000-8000-000000000001", "user@example.org", hash, "Test User", "00000000-0000-7000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}

	token, err := sessions.Create(context.Background(), auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "user@example.org", Name: "Test User", Role: auth.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		if Principal(c) == nil {
			t.Fatal("principal missing from context")
		}
		return c.String(http.StatusOK, "ok")
	}, RequireAuth(sessions, testLogger()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(sessions.Cookie(token))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated request = %d, want 200", rec.Code)
	}
}

func TestRequirePolicyAllowsAndDenies(t *testing.T) {
	pe, err := policy.NewPolicyEngine(openTestDB(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	auditLogger := newTestAudit(t)

	for _, tc := range []struct {
		name string
		role auth.Role
		code int
	}{
		{"admin allowed", auth.RoleAdmin, http.StatusOK},
		{"patient denied", auth.RolePatient, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
				func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c echo.Context) error {
						c.Set(principalKey, &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: tc.role})
						return next(c)
					}
				},
				RequirePolicy(pe, auditLogger, testLogger(), "admin.view"))

			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("GET /admin = %d, want %d", rec.Code, tc.code)
			}
		})
	}
}

func TestRequirePolicyRedirectsWithoutPrincipal(t *testing.T) {
	pe, err := policy.NewPolicyEngine(openTestDB(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
		RequirePolicy(pe, newTestAudit(t), testLogger(), "admin.view"))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("GET /admin without principal = %d, want 302", rec.Code)
	}
}

func TestRequirePolicyDenialIsAudited(t *testing.T) {
	db := openTestDB(t)
	auditLogger, err := audit.NewLogger(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	pe, err := policy.NewPolicyEngine(openTestDB(t), testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
		func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Set(principalKey, &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: auth.RolePatient})
				return next(c)
			}
		},
		RequirePolicy(pe, auditLogger, testLogger(), "admin.view"))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin = %d, want 403", rec.Code)
	}

	var action, resource, result string
	if err := db.QueryRow(`SELECT action, resource, result FROM audit_log`).
		Scan(&action, &resource, &result); err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	if action != "authorize" || resource != "policy:admin.view" || result != types.ResultFailure.String() {
		t.Fatalf("unexpected audit row: %q %q %q", action, resource, result)
	}
}

func newTestAudit(t *testing.T) *audit.Logger {
	t.Helper()
	db := openTestDB(t)
	l, err := audit.NewLogger(db, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestNotFoundRedirectsAnonymous(t *testing.T) {
	db := openTestDB(t)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/no-such-page", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("anonymous unknown route = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != LoginPath+"?next=%2Fno-such-page" {
		t.Errorf("redirect = %q, want login with next", loc)
	}
}

func TestNotFoundAuthenticated(t *testing.T) {
	db := openTestDB(t)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role_id) VALUES (?, ?, ?, ?, ?)`,
		"01990000-0000-7000-8000-000000000001", "user@example.org", hash, "Test User", "00000000-0000-7000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Create(context.Background(), auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "user@example.org", Name: "Test User", Role: auth.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/no-such-page", nil)
	req.AddCookie(sessions.Cookie(token))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("authenticated unknown route = %d, want 404", rec.Code)
	}
}

func TestNotFoundPublicPaths(t *testing.T) {
	db := openTestDB(t)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown /setup path = %d, want 404 (public path)", rec.Code)
	}
}
