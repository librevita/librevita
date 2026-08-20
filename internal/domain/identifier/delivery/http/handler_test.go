package http_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/identifiersystem"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/vault"
	deliveryhttp "librevita.org/internal/domain/identifier/delivery/http"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierrepo "librevita.org/internal/domain/identifier/repository"
	"librevita.org/internal/domain/identifier/usecase"
	"librevita.org/internal/testutil"
)

func openTestDB(t *testing.T) *ent.Client {
	t.Helper()
	name := "identifier-http-test-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
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
	return client
}

func newHTTPEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *ent.Client) {
	t.Helper()
	client := openTestDB(t)
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(auth.NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(audit.NewAuditRepository(client), log)
	if err != nil {
		t.Fatal(err)
	}
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

	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	if err := testutil.Clinic(context.Background(), client, "01990000-0000-7000-8000-0000000000d0", "Test Clinic", "000.000.000-00"); err != nil {
		t.Fatalf("seed clinic: %v", err)
	}
	if err := testutil.User(context.Background(), client, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", "x"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	sysRepo := identifierrepo.NewSystemRepository(client)
	reg := identifiermodel.NewRegistry()
	rows, err := sysRepo.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatal(err)
	}
	systemsSvc := usecase.NewSystemsService(sysRepo, reg, log)
	h := deliveryhttp.NewHandler(systemsSvc, csrf, auditLogger)

	e := echo.New()
	admin := []echo.MiddlewareFunc{
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "admin.view"),
	}
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)
	return e, sessions, client
}

var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

func adminSession(t *testing.T, sessions *auth.SessionManager) *http.Cookie {
	t.Helper()
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sessions.Cookie(token)
}

func postForm(t *testing.T, e *echo.Echo, path string, cookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func getWithCookie(t *testing.T, e *echo.Echo, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestIdentifierSystemsAdmin(t *testing.T) {
	e, sessions, db := newHTTPEnv(t)
	cookie := adminSession(t, sessions)

	// Create a Paraguayan cédula.
	rec := postForm(t, e, "/identifier-systems", cookie, url.Values{
		"system":             {"urn:librevita:id:py:cedula"},
		"display_name":       {"Cédula de Identidad (Paraguay)"},
		"pattern":            {"[0-9]{8}"},
		"transform":          {"digits"},
		"check_algorithm":    {"mod11_desc"},
		"check_base_len":     {"7"},
		"check_dv_count":     {"1"},
		"check_start_weight": {"8"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("create status = %d, want 302 (%s)", rec.Code, rec.Body.String())
	}

	// The catalog lists it.
	page := getWithCookie(t, e, "/identifier-systems", cookie)
	if !strings.Contains(page.Body.String(), "Cédula de Identidad") {
		t.Fatalf("catalog page = %q, want the new system", page.Body.String())
	}

	// Invalid regex is rejected inline.
	bad := postForm(t, e, "/identifier-systems", cookie, url.Values{
		"system":          {"urn:librevita:id:x:bad"},
		"display_name":    {"Bad"},
		"pattern":         {"["},
		"transform":       {"none"},
		"check_algorithm": {"none"},
	})
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), "not a valid regex") {
		t.Fatalf("bad regex status = %d body = %q, want inline error", bad.Code, bad.Body.String())
	}

	// URN outside the namespace is rejected.
	outside := postForm(t, e, "/identifier-systems", cookie, url.Values{
		"system":          {"com.example:doc"},
		"display_name":    {"Outside"},
		"pattern":         {"[0-9]{8}"},
		"transform":       {"none"},
		"check_algorithm": {"none"},
	})
	if outside.Code != http.StatusOK || !strings.Contains(outside.Body.String(), "urn:librevita:id:") {
		t.Fatalf("outside status = %d body = %q, want namespace error", outside.Code, outside.Body.String())
	}

	// Toggle the cédula inactive: the registry reloads.
	sysRow, err := db.IdentifierSystem.Query().
		Where(identifiersystem.SystemEQ("urn:librevita:id:py:cedula")).
		First(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	systemID := sysRow.ID.String()
	toggle := postForm(t, e, "/identifier-systems/"+systemID+"/active", cookie, url.Values{})
	if toggle.Code != http.StatusOK || !strings.Contains(toggle.Body.String(), "Inactive") {
		t.Fatalf("toggle status = %d body = %q, want the inactive row", toggle.Code, toggle.Body.String())
	}

	// Check fields partial renders only for a chosen algorithm.
	fields := getWithCookie(t, e, "/identifier-systems/check-fields?check_algorithm=mod11_desc", cookie)
	if !strings.Contains(fields.Body.String(), "Start weight") {
		t.Fatalf("fields partial = %q, want the start weight input", fields.Body.String())
	}
	none := getWithCookie(t, e, "/identifier-systems/check-fields?check_algorithm=none", cookie)
	if strings.Contains(none.Body.String(), "Start weight") {
		t.Fatalf("none partial = %q, must not render the check fields", none.Body.String())
	}
}
