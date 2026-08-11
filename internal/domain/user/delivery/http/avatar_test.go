package http

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/user/usecase"
)

// testAdminID is the seeded admin used by the avatar fixtures.
var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

// mustFileManager builds a FileManager over a temp local store.
func mustFileManager(t *testing.T, db *sql.DB) *storage.FileManager {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fm, err := storage.NewFileManager(db, s, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return fm
}

func newAvatarEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *storage.FileManager, *sql.DB) {
	t.Helper()
	db := openAvatarDB(t)
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, log)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(db, log)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.NewPolicyEngine(db, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := policies.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewService(db, sessions, auditLogger, log)
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role_id)
		VALUES ('01990000-0000-7000-8000-00000000000a', 'admin@example.org', 'x', 'Admin',
		(SELECT id FROM roles WHERE name = 'admin'))`); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	files := mustFileManager(t, db)
	csrf := auth.NewCSRF(&config.Config{Env: "development"})
	h := NewHandler(svc, nil, csrf, sessions, policies, auditLogger, clinic.NewClockProvider(db), files, log)

	e := echo.New()
	e.GET("/profile/avatar", h.Avatar, server.RequireAuth(sessions, log))
	e.POST("/profile/avatar", h.AvatarUpload,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "profile.update"))
	e.POST("/profile/avatar/remove", h.AvatarRemove,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "profile.update"))
	return e, sessions, files, db
}

func openAvatarDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "avatar.db"))
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

// a tiny valid PNG (1x1) and its sniffed content type.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func avatarMultipart(t *testing.T, field, name string, content []byte, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	if err := w.WriteField("content_type_hint", contentType); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

// TestAvatarUploadServeRemove covers the one-avatar-per-user flow: the
// second upload replaces the first, the served image is the newest, and
// remove clears it back to the placeholder.
func TestAvatarUploadServeRemove(t *testing.T) {
	e, sessions, files, db := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)
	ctx := context.Background()

	// Upload 1.
	buf, ctype := avatarMultipart(t, "avatar", "a.png", tinyPNG, "image/png")
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	req.Header.Set("Content-Type", ctype)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("upload status = %d, want 302", rec.Code)
	}
	// Upload 2 replaces it.
	buf2, ctype2 := avatarMultipart(t, "avatar", "b.png", tinyPNG, "image/png")
	req2 := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf2)
	req2.Header.Set("Content-Type", ctype2)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("second upload status = %d, want 302", rec2.Code)
	}

	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM storage_objects`).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Errorf("storage_objects rows = %d, want 1 (only the newest avatar)", cnt)
	}
	avatars, err := files.List(ctx, "avatar", testAdminID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("avatars len=%d err=%v", len(avatars), err)
	if len(avatars) != 1 {
		t.Fatalf("avatars after two uploads = %d, want 1", len(avatars))
	}

	// Serve: real image, not the placeholder.
	srv := httptest.NewRequest(http.MethodGet, "/profile/avatar", nil)
	srv.AddCookie(cookie)
	srec := httptest.NewRecorder()
	e.ServeHTTP(srec, srv)
	if srec.Code != http.StatusOK {
		t.Fatalf("serve status = %d", srec.Code)
	}
	if ct := srec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Errorf("served content type = %q, want image/*", ct)
	}
	if strings.Contains(srec.Body.String(), "<svg") {
		t.Error("served the placeholder despite an uploaded avatar")
	}

	// Remove: back to the placeholder (SVG, always 200).
	rm := httptest.NewRequest(http.MethodPost, "/profile/avatar/remove", nil)
	rm.AddCookie(cookie)
	rrec := httptest.NewRecorder()
	e.ServeHTTP(rrec, rm)
	if rrec.Code != http.StatusFound {
		t.Fatalf("remove status = %d, want 302", rrec.Code)
	}
	srv2 := httptest.NewRequest(http.MethodGet, "/profile/avatar", nil)
	srv2.AddCookie(cookie)
	srec2 := httptest.NewRecorder()
	e.ServeHTTP(srec2, srv2)
	if srec2.Code != http.StatusOK || !strings.Contains(srec2.Body.String(), "<svg") {
		t.Errorf("serve after remove = %d, want placeholder SVG", srec2.Code)
	}
}

// TestAvatarRejectsNonImage asserts the sniffed content type gate.
func TestAvatarRejectsNonImage(t *testing.T) {
	e, sessions, _, _ := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)

	buf, ctype := avatarMultipart(t, "avatar", "evil.txt", []byte("#!/bin/sh\nrm -rf /"), "text/plain")
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	req.Header.Set("Content-Type", ctype)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-image upload status = %d, want 400", rec.Code)
	}
}
