package http

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/patient/identifier"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/testutil"
)

var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

// newIdentifierServices wires the identifier subsystem against a
// migrated database: a fixed master key, the registry seeded from the
// migration rows, and the two services the handlers use.
func newIdentifierServices(t *testing.T, db *sql.DB, log *slog.Logger) (*identifier.Service, *identifier.SystemsService) {
	t.Helper()
	key, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=")
	if err != nil {
		t.Fatal(err)
	}
	reg := identifier.NewRegistry()
	rows, err := identifier.LoadActiveSystems(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatal(err)
	}
	return identifier.NewService(db, key, reg, log), identifier.NewSystemsService(db, reg, log)
}

func newDocEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *usecase.Service, *storage.FileManager) {
	e, sessions, svc, files, _ := newDocEnvFull(t, t.TempDir())
	return e, sessions, svc, files
}

// newDocEnvFull is newDocEnv with the blob directory and the database
// exposed, so the tests can tamper with stored objects and inspect the
// audit trail.
func newDocEnvFull(t *testing.T, dir string) (*echo.Echo, *auth.SessionManager, *usecase.Service, *storage.FileManager, *sql.DB) {
	t.Helper()
	db := openDocDB(t)
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(db, &config.Config{Mode: "development"}, log)
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
	svc := usecase.NewService(db, log, policies)
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := storage.NewFileManager(db, store, log)
	if err != nil {
		t.Fatal(err)
	}
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	if err := testutil.Clinic(context.Background(), db, "01990000-0000-7000-8000-0000000000d0", "Test Clinic", "000.000.000-00"); err != nil {
		t.Fatalf("seed clinic: %v", err)
	}
	if err := testutil.User(context.Background(), db, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", "x"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	ids, systems := newIdentifierServices(t, db, log)
	h := NewHandler(svc, clinic.NewClockProvider(db), csrf, auditLogger, files, ids, systems)

	e := echo.New()
	e.POST("/patients/:id/documents", h.UploadDocument,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.write"))
	e.GET("/patients/:id/documents/:fileID", h.DownloadDocument,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.read"))
	return e, sessions, svc, files, db
}

func openDocDB(t *testing.T) *sql.DB {
	t.Helper()
	name := "patient-docs-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
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

func mustLocalStore(t *testing.T) storage.Store {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

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

func newPatient(t *testing.T, svc *usecase.Service, clinicID string) uuid.UUID {
	t.Helper()
	pt, err := svc.Create(context.Background(), clinicID, testAdminID.String(), usecase.PatientInput{
		DisplayName: "Ana Souza", Sex: "female",
	})
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	return pt.ID
}

// TestDocumentsUploadDownloadIDOR covers the full attachment flow: an
// admin uploads a file to a patient, downloads it back, and a request
// for the same file under another patient's id is refused (IDOR).
func TestDocumentsUploadDownloadIDOR(t *testing.T) {
	e, sessions, svc, files := newDocEnv(t)
	cookie := adminSession(t, sessions)
	clinicID := "01990000-0000-7000-8000-0000000000d0"
	patientID := newPatient(t, svc, clinicID)
	otherID := newPatient(t, svc, clinicID)

	// Upload via multipart form.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "laudo.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("pdf-data"))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("upload status = %d, want 302", rec.Code)
	}

	meta, err := files.List(context.Background(), "patient_document", patientID)
	if err != nil || len(meta) != 1 {
		t.Fatalf("index after upload = %v, %v; want 1 file", meta, err)
	}
	if meta[0].OriginalName != "laudo.pdf" {
		t.Errorf("stored name = %q, want laudo.pdf", meta[0].OriginalName)
	}

	// Download from the owning patient.
	dl := httptest.NewRequest(http.MethodGet,
		"/patients/"+patientID.String()+"/documents/"+meta[0].ID.String(), nil)
	dl.AddCookie(cookie)
	drec := httptest.NewRecorder()
	e.ServeHTTP(drec, dl)
	if drec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", drec.Code)
	}
	if got := drec.Body.String(); got != "pdf-data" {
		t.Errorf("download body = %q, want pdf-data", got)
	}
	if cd := drec.Header().Get("Content-Disposition"); !strings.Contains(cd, "laudo.pdf") {
		t.Errorf("Content-Disposition = %q, want the original name", cd)
	}

	// IDOR: the same file id under another patient must 404.
	idl := httptest.NewRequest(http.MethodGet,
		"/patients/"+otherID.String()+"/documents/"+meta[0].ID.String(), nil)
	idl.AddCookie(cookie)
	irec := httptest.NewRecorder()
	e.ServeHTTP(irec, idl)
	if irec.Code != http.StatusNotFound {
		t.Errorf("IDOR status = %d, want 404", irec.Code)
	}

	// Unknown file id under the owning patient also 404s.
	miss := httptest.NewRequest(http.MethodGet,
		"/patients/"+patientID.String()+"/documents/"+uuid.Must(uuid.NewV7()).String(), nil)
	miss.AddCookie(cookie)
	mrec := httptest.NewRecorder()
	e.ServeHTTP(mrec, miss)
	if mrec.Code != http.StatusNotFound {
		t.Errorf("unknown file status = %d, want 404", mrec.Code)
	}
}

// TestDocumentsDownloadDetectsTampering uploads a file, corrupts the
// blob on disk and checks that the download still streams but the
// divergence is registered in the append-only audit trail.
func TestDocumentsDownloadDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	e, sessions, svc, files, db := newDocEnvFull(t, dir)
	cookie := adminSession(t, sessions)
	clinicID := "01990000-0000-7000-8000-0000000000d0"
	patientID := newPatient(t, svc, clinicID)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "laudo.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("pdf-data"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("upload status = %d, want 302", rec.Code)
	}

	meta, err := files.List(context.Background(), "patient_document", patientID)
	if err != nil || len(meta) != 1 {
		t.Fatalf("index after upload = %v, %v; want 1 file", meta, err)
	}

	// Tamper with the blob on disk, behind the application's back.
	blobPath := filepath.Join(dir, filepath.FromSlash(meta[0].Key))
	if err := os.WriteFile(blobPath, []byte("tampered!"), 0o600); err != nil {
		t.Fatal(err)
	}

	dl := httptest.NewRequest(http.MethodGet,
		"/patients/"+patientID.String()+"/documents/"+meta[0].ID.String(), nil)
	dl.AddCookie(cookie)
	drec := httptest.NewRecorder()
	e.ServeHTTP(drec, dl)
	if drec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", drec.Code)
	}
	if got := drec.Body.String(); got != "tampered!" {
		t.Errorf("download body = %q, want the tampered payload", got)
	}

	var detail string
	err = db.QueryRowContext(context.Background(),
		`SELECT detail FROM audit_log WHERE action = 'file.read' AND result = 'failure' ORDER BY id DESC LIMIT 1`).Scan(&detail)
	if err != nil {
		t.Fatalf("query audit trail: %v", err)
	}
	if !strings.Contains(detail, "checksum mismatch") {
		t.Errorf("audit detail = %q, want checksum mismatch", detail)
	}
}

// TestDocumentsUploadRequiresAuth rejects anonymous uploads.
func TestDocumentsUploadRequiresAuth(t *testing.T) {
	e, _, _, _ := newDocEnv(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "x.pdf")
	_, _ = fw.Write([]byte("x"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/patients/01990000-0000-7000-8000-0000000000d0/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound && rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous upload status = %d, want redirect to login", rec.Code)
	}
}
