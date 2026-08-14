package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/testutil"
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
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, testLogger())
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
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.User(context.Background(), db, "01990000-0000-7000-8000-000000000001", "user@example.org", "admin", hash); err != nil {
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
	if action != "authorize" || resource != "policy:admin.view" || result != types.AuditResultFailure.String() {
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
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, testLogger())
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
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.User(context.Background(), db, "01990000-0000-7000-8000-000000000001", "user@example.org", "admin", hash); err != nil {
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
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, testLogger())
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

// TestBodyLimitRaisesAvatarUploadCap pins the effective upload ceiling:
// the global 1M body limit skips the avatar and document routes, so
// their own per-route limits (2M avatar, 25M documents) are reachable.
func TestSecurityHeadersAreStrict(t *testing.T) {
	e := New(auth.NewCSRF(&config.Config{Mode: "development"}), &config.Config{Mode: "development"})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	const wantCSP = "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	if got := rec.Header().Get("Content-Security-Policy"); got != wantCSP {
		t.Fatalf("CSP = %q, want %q", got, wantCSP)
	}
	for _, h := range []string{
		"X-Content-Type-Options",
		"Referrer-Policy",
		"X-Frame-Options",
		"Cross-Origin-Resource-Policy",
		"Permissions-Policy",
	} {
		if rec.Header().Get(h) == "" {
			t.Fatalf("missing security header %s", h)
		}
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS present with hsts_max_age=0: %q", got)
	}
}

func TestSecurityHeadersHSTSIsConfigurable(t *testing.T) {
	e := New(auth.NewCSRF(&config.Config{Mode: "development"}), &config.Config{Mode: "development", HSTSMaxAge: 31536000})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("HSTS = %q, want max-age=31536000", got)
	}
}

func TestBodyLimitRaisesAvatarUploadCap(t *testing.T) {
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	e := New(csrf, &config.Config{Mode: "development"})
	e.POST("/profile/avatar", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.POST("/profile", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	big := strings.Repeat("x", 1<<20+1024)

	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", strings.NewReader(big))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatal("avatar upload was capped by the global 1M body limit")
	}

	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(big))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("plain route status = %d, want 413 from the global limit", rec.Code)
	}
}
