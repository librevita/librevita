package http

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	userrepo "librevita.org/internal/domain/user/repository"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/testutil"
	"librevita.org/pkg/log"
)

// testAdminID is the seeded admin used by the avatar fixtures.
var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

// mustFileManager builds a FileManager over a temp local store.
func mustFileManager(t *testing.T, client *ent.Client) *storage.FileManager {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fm, err := storage.NewFileManager(storage.NewIndexRepository(client), s, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	return fm
}

func newAvatarEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *storage.FileManager, *ent.Client) {
	t.Helper()
	client := openAvatarDB(t)
	logger := log.Nop()
	sessions, err := auth.NewSessionManager(auth.NewSessionRepository(client), &config.Config{Mode: "development"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(audit.NewAuditRepository(client), logger)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := policy.NewPolicyEngine(policy.NewPolicyRepository(client), logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := policies.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	userRepo := userrepo.NewUserRepository(client)
	roleRepo := userrepo.NewRoleRepository(client)
	specialtyRepo := userrepo.NewSpecialtyRepository(client)
	staffReqRepo := userrepo.NewStaffRequestRepository(client)
	setupRepo := userrepo.NewSetupRepository(client)

	svc := usecase.NewService(userRepo, roleRepo, specialtyRepo, staffReqRepo, setupRepo, sessions, auditLogger, logger)
	if err := testutil.User(context.Background(), client, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", "x"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	files := mustFileManager(t, client)
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	h := NewHandler(svc, nil, nil, nil, csrf, sessions, policies, auditLogger, clinicusecase.NewClockProvider(clinicrepo.NewClinicRepository(client)), files, &config.Config{Mode: "development"}, logger)

	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := clinicctx.WithTestClinic(c.Request().Context())
			ctx = fle.WithClinicID(ctx, clinicctx.TestClinicID.String())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/profile/avatar", h.Avatar, server.RequireAuth(sessions, logger))
	e.POST("/profile/avatar", h.AvatarUpload,
		server.RequireAuth(sessions, logger),
		server.RequirePolicy(policies, auditLogger, logger, "profile.update"))
	e.POST("/profile/avatar/remove", h.AvatarRemove,
		server.RequireAuth(sessions, logger),
		server.RequirePolicy(policies, auditLogger, logger, "profile.update"))
	return e, sessions, files, client
}

func openAvatarDB(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "avatar.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db, log.Nop()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })
	return client
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
	_, _ = fw.Write(content)
	if err := w.WriteField("content_type_hint", contentType); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
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

	cnt, err := db.StorageObject.Query().Count(ctx)
	if err != nil {
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

// TestAvatarCacheContract pins the caching behavior: both the real
// picture and the placeholder are private and revalidated before every
// use (no-cache), and a validating request (If-None-Match) gets a 304
// instead of the body — so an upload reflects on the next page load.
func TestAvatarCacheContract(t *testing.T) {
	e, sessions, _, _ := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)

	get := func(etag string) (*httptest.ResponseRecorder, string) {
		srv := httptest.NewRequest(http.MethodGet, "/profile/avatar", nil)
		srv.AddCookie(cookie)
		if etag != "" {
			srv.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, srv)
		return rec, rec.Header().Get("ETag")
	}

	// Placeholder: private + no-cache, ETag present, 200 on first fetch.
	rec, etag := get("")
	if rec.Code != http.StatusOK {
		t.Fatalf("placeholder status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Errorf("placeholder cache-control = %q", cc)
	}
	if etag == "" {
		t.Fatal("placeholder has no ETag")
	}
	if unquoted := strings.Trim(etag, `"`); strings.HasPrefix(unquoted, "ph-") == false || len(strings.TrimPrefix(unquoted, "ph-")) != 64 {
		t.Errorf("placeholder ETag = %q, want a quoted ph- + sha256 pair", etag)
	}

	// A validating fetch is answered with 304 and no body.
	rec, _ = get(etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("placeholder revalidated status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Error("304 carried a body")
	}
	// A mismatching tag must fetch the payload again.
	rec, _ = get(`"other"`)
	if rec.Code != http.StatusOK {
		t.Fatalf("placeholder with stale tag = %d, want 200", rec.Code)
	}

	// Upload a real picture and repeat the contract with its ETag.
	buf, ctype := avatarMultipart(t, "avatar", "a.png", tinyPNG, "image/png")
	up := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	up.Header.Set("Content-Type", ctype)
	up.AddCookie(cookie)
	uprec := httptest.NewRecorder()
	e.ServeHTTP(uprec, up)
	if uprec.Code != http.StatusFound {
		t.Fatalf("upload status = %d, want 302", uprec.Code)
	}

	rec, etag = get("")
	if rec.Code != http.StatusOK {
		t.Fatalf("picture status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Errorf("picture cache-control = %q", cc)
	}
	if etag == "" || strings.HasPrefix(strings.Trim(etag, `"`), "ph-") {
		t.Errorf("picture ETag = %q, want a storage tag, not the placeholder's", etag)
	}
	rec, _ = get(etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("picture revalidated status = %d, want 304", rec.Code)
	}
	// Weak validation (W/"tag") must match too, per RFC 9110.
	rec, _ = get("W/" + etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("picture with weak tag = %d, want 304", rec.Code)
	}
}

// tinyBMP is a minimal 1x1 24bpp bitmap (54-byte header, one padded
// row of three blue pixel bytes), for the supplementary-decoder test.
var tinyBMP = func() []byte {
	b := make([]byte, 58)
	copy(b, "BM")
	put32(b[2:], 58)                          // file size
	put32(b[10:], 54)                         // pixel data offset
	put32(b[14:], 40)                         // DIB header size
	put32(b[18:], 1)                          // width
	put32(b[22:], 1)                          // height
	put16(b[26:], 1)                          // planes
	put16(b[28:], 24)                         // bits per pixel
	b[54], b[55], b[56], b[57] = 0, 0, 255, 0 // B G R + row padding
	return b
}()

func put16(b []byte, v uint16) {
	// #nosec G115 -- intentional byte truncation while hand-crafting a
	// BMP fixture header.
	b[0], b[1] = byte(v), byte(v>>8)
}

func put32(b []byte, v uint32) {
	// #nosec G115 -- intentional byte truncation while hand-crafting a
	// BMP fixture header.
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

// TestAvatarAcceptsSupplementaryFormats pins the extra decoders: bmp
// and webp pass the sniffed gate and are processed down to the same
// 256x256 JPEG as every other source.
func TestAvatarAcceptsSupplementaryFormats(t *testing.T) {
	// A real webp is not hand-craftable here; the gate + decoder
	// registration is the same mechanism as bmp, so the bitmap fixture
	// pins the whole pipeline (sniff, decode, thumbnail, re-encode).
	e, sessions, _, _ := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)

	buf, ctype := avatarMultipart(t, "avatar", "pixel.bmp", tinyBMP, "image/bmp")
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	req.Header.Set("Content-Type", ctype)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("bmp upload status = %d, want 302", rec.Code)
	}

	srv := httptest.NewRequest(http.MethodGet, "/profile/avatar", nil)
	srv.AddCookie(cookie)
	srec := httptest.NewRecorder()
	e.ServeHTTP(srec, srv)
	if srec.Code != http.StatusOK {
		t.Fatalf("serve status = %d", srec.Code)
	}
	img, format, err := image.Decode(bytes.NewReader(srec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response is not a decodable image: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("response format = %q, want jpeg", format)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("response size = %dx%d, want 256x256", b.Dx(), b.Dy())
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

// pngWithSize builds a minimal valid PNG declaring the given
// dimensions, for the decompression-bomb test.
func pngWithSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	// #nosec G115 -- intentional byte truncation while hand-crafting a
	// PNG fixture.
	sig := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	ihdr := make([]byte, 13)
	ihdr[0], ihdr[1], ihdr[2], ihdr[3] = byte(w>>24), byte(w>>16), byte(w>>8), byte(w) // #nosec G115 -- PNG fixture bytes
	ihdr[4], ihdr[5], ihdr[6], ihdr[7] = byte(h>>24), byte(h>>16), byte(h>>8), byte(h) // #nosec G115 -- PNG fixture bytes
	ihdr[8], ihdr[9], ihdr[10], ihdr[11], ihdr[12] = 8, 2, 0, 0, 0
	chunk := func(tag string, data []byte) []byte {
		b := make([]byte, 8+len(data)+4)
		b[0], b[1], b[2], b[3] = byte(len(data)>>24), byte(len(data)>>16), byte(len(data)>>8), byte(len(data)) // #nosec G115 -- PNG fixture bytes
		copy(b[4:], tag)
		copy(b[8:], data)
		crc := crc32IEEE(tag, data)
		b[len(b)-4] = byte(crc >> 24) // #nosec G115 -- PNG fixture bytes
		b[len(b)-3] = byte(crc >> 16) // #nosec G115 -- PNG fixture bytes
		b[len(b)-2] = byte(crc >> 8)  // #nosec G115 -- PNG fixture bytes
		b[len(b)-1] = byte(crc)       // #nosec G115 -- PNG fixture bytes
		return b
	}
	out := append(sig, chunk("IHDR", ihdr)...)
	idat := []byte{0x78, 0x9c, 0x63, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
	out = append(out, chunk("IDAT", idat)...)
	return append(out, chunk("IEND", nil)...)
}

func crc32IEEE(tag string, data []byte) uint32 {
	var table [256]uint32
	for n := 0; n < 256; n++ {
		c := uint32(n)
		for k := 0; k < 8; k++ {
			if c&1 == 1 {
				c = 0xedb88320 ^ (c >> 1)
			} else {
				c >>= 1
			}
		}
		table[n] = c
	}
	crc := ^uint32(0)
	for _, b := range append([]byte(tag), data...) {
		crc = table[crc&0xff^uint32(b)] ^ (crc >> 8)
	}
	return ^crc
}

// TestAvatarProcessing asserts the upload pipeline: any accepted image
// is served back as a 256x256 JPEG, not the original bytes.
func TestAvatarProcessing(t *testing.T) {
	e, sessions, _, _ := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)

	buf, ctype := avatarMultipart(t, "avatar", "photo.png", tinyPNG, "image/png")
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	req.Header.Set("Content-Type", ctype)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("upload status = %d, want 302", rec.Code)
	}

	srv := httptest.NewRequest(http.MethodGet, "/profile/avatar", nil)
	srv.AddCookie(cookie)
	srec := httptest.NewRecorder()
	e.ServeHTTP(srec, srv)
	if ct := srec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("served content type = %q, want image/jpeg", ct)
	}
	img, format, err := image.Decode(bytes.NewReader(srec.Body.Bytes()))
	if err != nil {
		t.Fatalf("response is not a decodable image: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("response format = %q, want jpeg", format)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("response size = %dx%d, want 256x256", b.Dx(), b.Dy())
	}
}

// TestAvatarRejectsHugeDimensions asserts the decompression-bomb guard:
// a tiny PNG declaring a huge canvas is refused before any decode.
func TestAvatarRejectsHugeDimensions(t *testing.T) {
	e, sessions, _, _ := newAvatarEnv(t)
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessions.Cookie(token)

	bomb := pngWithSize(t, 30000, 30000)
	buf, ctype := avatarMultipart(t, "avatar", "bomb.png", bomb, "image/png")
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", buf)
	req.Header.Set("Content-Type", ctype)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("huge image status = %d, want 400", rec.Code)
	}
}
