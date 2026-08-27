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

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/auditlog"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/core/vault"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierrepo "librevita.org/internal/domain/identifier/repository"
	identifierusecase "librevita.org/internal/domain/identifier/usecase"
	patientrepo "librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/testutil"
)

var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

// newIdentifierServices wires the identifier subsystem against a
// migrated database: a fixed master key, the registry seeded from the
// migration rows, and the two services the handlers use.
func newIdentifierServices(t *testing.T, client *ent.Client, key *crypto.MasterKey, log *slog.Logger) (identifierusecase.Service, identifierusecase.SystemsService) {
	t.Helper()
	reg := identifiermodel.NewRegistry()
	sysRepo := identifierrepo.NewSystemRepository(client)
	rows, err := sysRepo.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatal(err)
	}
	idRepo := identifierrepo.NewIdentifierRepository(client)
	return identifierusecase.NewService(idRepo, key, reg, log), identifierusecase.NewSystemsService(sysRepo, reg, log)
}

func newDocEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *usecase.Service, *storage.FileManager) {
	e, sessions, svc, files, _ := newDocEnvFull(t, t.TempDir())
	return e, sessions, svc, files
}

// newDocEnvFull is newDocEnv with the blob directory and the database
// exposed, so the tests can tamper with stored objects and inspect the
// audit trail.
func newDocEnvFull(t *testing.T, dir string) (*echo.Echo, *auth.SessionManager, *usecase.Service, *storage.FileManager, *ent.Client) {
	t.Helper()
	client := openDocDB(t)
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
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := storage.NewFileManager(storage.NewIndexRepository(client), store, log)
	if err != nil {
		t.Fatal(err)
	}
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	if err := testutil.Clinic(context.Background(), client, "01990000-0000-7000-8000-0000000000d0", "Test Clinic", "000.000.000-00"); err != nil {
		t.Fatalf("seed clinic: %v", err)
	}
	if err := testutil.User(context.Background(), client, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", "x"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	masterKey, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v)
	if err != nil {
		t.Fatal(err)
	}
	clinicID := uuid.MustParse(testClinic)
	clinicDEK, err := masterKey.EnsureClinicDEK(context.Background(), clinicID)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.NewEncryptor(clinicDEK)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := crypto.NewHasherFromDEK(clinicDEK)
	crypto.ZeroBytes(clinicDEK)
	if err != nil {
		t.Fatal(err)
	}
	client.Use(ent.FLEMutationHook(hasher, enc, masterKey))
	client.Intercept(ent.FLEDecryptionInterceptor(enc, masterKey))
	svc := usecase.NewService(patientrepo.NewPatientRepositoryWithEngine(client, masterKey), log, policies, masterKey)
	ids, systems := newIdentifierServices(t, client, masterKey, log)
	h := NewHandler(svc, clinicusecase.NewClockProvider(clinicrepo.NewClinicRepository(client)), csrf, auditLogger, files, ids, systems, masterKey)

	e := echo.New()
	e.Use(attachSeededClinic(masterKey, enc, hasher))
	e.POST("/patients/:id/documents", h.UploadDocument,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.write"))
	e.GET("/patients/:id/documents/:fileID", h.DownloadDocument,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.document.read"))
	e.POST("/patients/:id/shred", h.Shred,
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.erase"))
	return e, sessions, svc, files, client
}

func openDocDB(t *testing.T) *ent.Client {
	t.Helper()
	name := "patient-docs-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })
	return client
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
		DisplayName: "Ana Souza",
		Phone:       "+55 11 99999-8888",
		Email:       "ana@example.org",
		Sex:         "female",
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

func TestDocumentsShredRemovesBlobAndPatient(t *testing.T) {
	e, sessions, svc, files := newDocEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, "01990000-0000-7000-8000-0000000000d0")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "laudo.pdf")
	require.NoError(t, err)
	_, err = fw.Write([]byte("pdf-data"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	filesBefore, err := files.List(context.Background(), "patient_document", patientID)
	require.NoError(t, err)
	require.Len(t, filesBefore, 1)

	shred := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/shred", nil)
	shred.AddCookie(cookie)
	shredRec := httptest.NewRecorder()
	e.ServeHTTP(shredRec, shred)
	require.Equal(t, http.StatusFound, shredRec.Code)

	filesAfter, err := files.List(context.Background(), "patient_document", patientID)
	require.NoError(t, err)
	assert.Empty(t, filesAfter)
	_, err = svc.Get(context.Background(), "01990000-0000-7000-8000-0000000000d0", patientID.String())
	assert.ErrorIs(t, err, usecase.ErrNotFound)
}

// TestDocumentsDownloadDetectsTampering uploads a file, corrupts the
// blob on disk and checks that the encrypted object is rejected and the
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
	if drec.Code != http.StatusInternalServerError {
		t.Fatalf("download status = %d, want 500", drec.Code)
	}
	if got := drec.Body.String(); strings.Contains(got, "tampered!") {
		t.Errorf("download body exposed the tampered payload: %q", got)
	}

	lastAudit, err := db.AuditLog.Query().
		Where(auditlog.ActionEQ("file.read"), auditlog.ResultEQ("failure")).
		Order(ent.Desc(auditlog.FieldID)).
		First(context.Background())
	if err != nil {
		t.Fatalf("query audit trail: %v", err)
	}
	if lastAudit.Detail == nil || !strings.Contains(*lastAudit.Detail, "encrypted object unavailable") {
		t.Errorf("audit detail = %v, want encrypted object unavailable", lastAudit.Detail)
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
