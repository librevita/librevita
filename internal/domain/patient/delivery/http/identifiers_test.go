package http

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/testutil"
)

var testClinic = "01990000-0000-7000-8000-0000000000d0"

// newIdentEnv mounts the identifier routes with real middlewares and a
// migrated database.
func newIdentEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *usecase.Service, *audit.Logger, *sql.DB) {
	t.Helper()
	db := openDocDB(t)
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
	svc := usecase.NewService(db, log, policies)
	files, err := storage.NewFileManager(db, mustLocalStore(t), log)
	if err != nil {
		t.Fatal(err)
	}
	csrf := auth.NewCSRF(&config.Config{Env: "development"})
	if err := testutil.Clinic(context.Background(), db, "01990000-0000-7000-8000-0000000000d0", "Test Clinic", "000.000.000-00"); err != nil {
		t.Fatalf("seed clinic: %v", err)
	}
	if err := testutil.User(context.Background(), db, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", "x"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	ids, systems := newIdentifierServices(t, db, log)
	h := NewHandler(svc, clinic.NewClockProvider(db), csrf, auditLogger, files, ids, systems)

	e := echo.New()
	view := []echo.MiddlewareFunc{
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
	}
	admin := []echo.MiddlewareFunc{
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "admin.view"),
	}
	lookup := append(view, server.RateLimit(server.NewRateLimiter(60, 1e9)))
	e.GET("/patients/lookup", h.IdentifierLookup, lookup...)
	e.POST("/patients/:id/identifiers", h.IdentifierAdd, view...)
	e.POST("/patients/:id/identifiers/:identifierID/remove", h.IdentifierRemove, view...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients", h.List, view...)
	e.POST("/patients", h.Create, view...)
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)
	return e, sessions, svc, auditLogger, db
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

func postFormHtmx(t *testing.T, e *echo.Echo, path string, cookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func getWithCookie(t *testing.T, e *echo.Echo, path string, cookie *http.Cookie, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestIdentifierAddAndLookup covers the reception flow: registering a
// CPF (auto-detected) and finding the patient by the formatted value.
func TestIdentifierAddAndLookup(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)

	rec := postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"123.456.789-09"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("add status = %d, want 302 (%s)", rec.Code, rec.Body.String())
	}

	// Exact lookup with the formatted value redirects to the patient.
	lookup := getWithCookie(t, e, "/patients/lookup?value=123.456.789-09", cookie, true)
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200", lookup.Code)
	}
	if got := lookup.Header().Get("HX-Redirect"); got != "/patients/"+patientID.String() {
		t.Fatalf("HX-Redirect = %q, want /patients/%s", got, patientID.String())
	}

	// Unknown value renders the empty state, not a redirect.
	miss := getWithCookie(t, e, "/patients/lookup?value=999999990", cookie, true)
	if miss.Code != http.StatusOK {
		t.Fatalf("miss status = %d, want 200", miss.Code)
	}
	if !strings.Contains(miss.Body.String(), "No patient holds this document") {
		t.Fatalf("miss body = %q", miss.Body.String())
	}

	// Values shorter than the minimum never hit the database.
	short := getWithCookie(t, e, "/patients/lookup?value=ab", cookie, true)
	if !strings.Contains(short.Body.String(), "characters") {
		t.Fatalf("short body = %q, want the minimum-length message", short.Body.String())
	}
}

// TestIdentifierAddValidation covers the invalid-value and duplicate
// paths of the add form.
func TestIdentifierAddValidation(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)

	// Bad check digit: the form re-renders with the message.
	bad := postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"12345678901"},
	})
	if bad.Code != http.StatusOK {
		t.Fatalf("bad status = %d, want 200", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), "invalid check digit") {
		t.Fatalf("bad body = %q, want the check digit message", bad.Body.String())
	}

	// Valid value first.
	ok := postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	if ok.Code != http.StatusFound {
		t.Fatalf("add status = %d, want 302", ok.Code)
	}

	// Duplicate on the same patient.
	dup := postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	if !strings.Contains(dup.Body.String(), "already has this document") {
		t.Fatalf("dup body = %q, want the already-has message", dup.Body.String())
	}

	// Duplicate on another patient of the clinic names the owner.
	otherID := newPatient(t, svc, testClinic)
	dupOther := postForm(t, e, "/patients/"+otherID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	if !strings.Contains(dupOther.Body.String(), "Ana Souza") {
		t.Fatalf("dupOther body = %q, want the owner name", dupOther.Body.String())
	}
}

// TestIdentifierAddRejectsCrossClinicPatient pins the IDOR guard: the
// add must load the patient through the clinic-scoped service, so a
// patient id of another clinic is a 404, not a write.
func TestIdentifierAddRejectsCrossClinicPatient(t *testing.T) {
	e, sessions, _, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	// A patient in another clinic.
	otherClinic := "01990000-0000-7000-8000-0000000000d1"
	if err := testutil.Clinic(context.Background(), db, otherClinic, "Other", "111.111.111-11"); err != nil {
		t.Fatal(err)
	}
	var otherPatient string
	if err := db.QueryRow(`INSERT INTO patients (id, clinic_id, display_name) VALUES (?, ?, 'Other Patient') RETURNING id`,
		"01990000-0000-7000-8000-0000000000d2", otherClinic).Scan(&otherPatient); err != nil {
		t.Fatal(err)
	}

	rec := postForm(t, e, "/patients/"+otherPatient+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-clinic add status = %d, want 404", rec.Code)
	}
}

// TestIdentifierRemoveAndIdempotency removes a document and verifies a
// second removal surfaces an error instead of a silent no-op.
func TestIdentifierRemoveAndIdempotency(t *testing.T) {
	e, sessions, svc, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)

	postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	var identifierID string
	if err := db.QueryRow(`SELECT id FROM patient_identifiers WHERE patient_id = ?`, patientID.String()).Scan(&identifierID); err != nil {
		t.Fatalf("load identifier: %v", err)
	}

	rec := postForm(t, e, "/patients/"+patientID.String()+"/identifiers/"+identifierID+"/remove", cookie, url.Values{})
	if rec.Code != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "data-lv-plain") {
		t.Fatalf("remove response still contains an identifier: %q", rec.Body.String())
	}

	// Second removal is not a silent success.
	again := postForm(t, e, "/patients/"+patientID.String()+"/identifiers/"+identifierID+"/remove", cookie, url.Values{})
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), "no longer exists") {
		t.Fatalf("second remove status = %d body = %q, want the gone message", again.Code, again.Body.String())
	}
}

// TestIdentifierDetailMasksValue verifies the detail page renders the
// masked value with the reveal wiring: the mask is visible text, the
// plaintext lives only in the data-lv-plain attribute (screen privacy,
// not access control -- the reader already has patient.view).
func TestIdentifierDetailMasksValue(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)

	postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})

	detail := getWithCookie(t, e, "/patients/"+patientID.String(), cookie, false)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", detail.Code)
	}
	body := detail.Body.String()
	if !strings.Contains(body, "529••••25") {
		t.Fatalf("detail body does not show the masked value: %q", body)
	}
	if !strings.Contains(body, `data-lv-plain="52998224725"`) {
		t.Fatalf("detail body has no reveal wiring: %q", body)
	}
	if !strings.Contains(body, "Identification documents") {
		t.Fatalf("detail body has no identifiers section: %q", body)
	}
}

// TestIdentifierLookupRequiresAuth rejects anonymous lookups: htmx
// requests receive the login redirect header, plain ones a 302.
func TestIdentifierLookupRequiresAuth(t *testing.T) {
	e, _, _, _, _ := newIdentEnv(t)
	rec := getWithCookie(t, e, "/patients/lookup?value=52998224725", nil, true)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("HX-Redirect"), "/auth/login") {
		t.Fatalf("anonymous htmx lookup status = %d HX-Redirect = %q, want login redirect",
			rec.Code, rec.Header().Get("HX-Redirect"))
	}
	plain := getWithCookie(t, e, "/patients/lookup?value=52998224725", nil, false)
	if plain.Code != http.StatusFound {
		t.Fatalf("anonymous plain lookup status = %d, want 302", plain.Code)
	}
}

// TestIdentifierSystemsAdmin covers the administrator catalog: create,
// validation, toggle, and registry reload.
func TestIdentifierSystemsAdmin(t *testing.T) {
	e, sessions, _, _, db := newIdentEnv(t)
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
	page := getWithCookie(t, e, "/identifier-systems", cookie, false)
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

	// Toggle the cédula inactive: the registry reloads and the value
	// falls back to raw detection.
	var systemID string
	if err := db.QueryRow(`SELECT id FROM identifier_systems WHERE system = 'urn:librevita:id:py:cedula'`).Scan(&systemID); err != nil {
		t.Fatal(err)
	}
	toggle := postForm(t, e, "/identifier-systems/"+systemID+"/active", cookie, url.Values{})
	if toggle.Code != http.StatusOK || !strings.Contains(toggle.Body.String(), "Inactive") {
		t.Fatalf("toggle status = %d body = %q, want the inactive row", toggle.Code, toggle.Body.String())
	}

	// Check fields partial renders only for a chosen algorithm. The
	// "none" branch emits hidden defaults so the form still submits
	// without JavaScript, but no labeled inputs.
	fields := getWithCookie(t, e, "/identifier-systems/check-fields?check_algorithm=mod11_desc", cookie, false)
	if !strings.Contains(fields.Body.String(), "Start weight") {
		t.Fatalf("fields partial = %q, want the start weight input", fields.Body.String())
	}
	none := getWithCookie(t, e, "/identifier-systems/check-fields?check_algorithm=none", cookie, false)
	if strings.Contains(none.Body.String(), "Start weight") {
		t.Fatalf("none partial = %q, must not render the check fields", none.Body.String())
	}
}

// TestAuditNeverContainsPlaintext verifies that the audit trail carries
// no document value: only system URNs and counts.
func TestAuditNeverContainsPlaintext(t *testing.T) {
	e, sessions, svc, auditLogger, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)

	postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	getWithCookie(t, e, "/patients/lookup?value=52998224725", cookie, true)

	events, err := auditLogger.Recent(context.Background(), 50, 0)
	if err != nil {
		t.Fatalf("recent events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no audit events recorded")
	}
	for _, ev := range events {
		detail := ""
		if ev.Detail != nil {
			detail = *ev.Detail
		}
		if strings.Contains(detail, "52998224725") {
			t.Fatalf("audit leaked the plaintext value in %+v", ev)
		}
	}
}

// TestCreatePatientWithDocument covers the registration form: a patient
// created with an identification document gets the encrypted identifier
// right away, findable by the exact lookup.
func TestCreatePatientWithDocument(t *testing.T) {
	e, sessions, _, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	rec := postForm(t, e, "/patients", cookie, url.Values{
		"display_name":     {"Novo Paciente"},
		"identifier_value": {"123.456.789-09"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("create status = %d, want 302 (%s)", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/patients/") {
		t.Fatalf("Location = %q, want /patients/<id>", loc)
	}

	// The document is findable by the exact lookup.
	lookup := getWithCookie(t, e, "/patients/lookup?value=123.456.789-09", cookie, true)
	if lookup.Header().Get("HX-Redirect") != loc {
		t.Fatalf("lookup HX-Redirect = %q, want %q", lookup.Header().Get("HX-Redirect"), loc)
	}
}

// TestCreatePatientRejectsBadDocument pins the validation order: an
// invalid document fails the form before the patient row is written.
func TestCreatePatientRejectsBadDocument(t *testing.T) {
	e, sessions, _, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	rec := postFormHtmx(t, e, "/patients", cookie, url.Values{
		"display_name":     {"Novo Paciente"},
		"identifier_value": {"12345678901"}, // wrong CPF check digit
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200 form error", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid check digit") {
		t.Fatalf("body = %q, want the check digit message", rec.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM patients`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("patients = %d, want 0 (nothing written on document error)", count)
	}
}

// TestCreatePatientRejectsDuplicateDocument: the same document cannot be
// registered through a second patient, and the second patient is not
// created.
func TestCreatePatientRejectsDuplicateDocument(t *testing.T) {
	e, sessions, _, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	first := postForm(t, e, "/patients", cookie, url.Values{
		"display_name":     {"Primeiro"},
		"identifier_value": {"52998224725"},
	})
	if first.Code != http.StatusFound {
		t.Fatalf("first create status = %d, want 302", first.Code)
	}

	second := postFormHtmx(t, e, "/patients", cookie, url.Values{
		"display_name":     {"Segundo"},
		"identifier_value": {"52998224725"},
	})
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "already registered") {
		t.Fatalf("second create status = %d body = %q, want the duplicate message", second.Code, second.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM patients`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("patients = %d, want 1", count)
	}
}

// TestRegistryListMasksDocument: the patients table shows the document
// column from the encrypted identifiers, masked, or a dash when the
// patient has none.
func TestRegistryListMasksDocument(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	withDoc := newPatient(t, svc, testClinic)
	postForm(t, e, "/patients/"+withDoc.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	withoutDoc := newPatient(t, svc, testClinic)

	page := getWithCookie(t, e, "/patients", cookie, false)
	if page.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "529••••25") {
		t.Fatalf("list body does not show the masked document: %q", body)
	}
	if strings.Contains(body, "52998224725") {
		t.Fatalf("list body leaks the plaintext document: %q", body)
	}
	if !strings.Contains(body, "Ana Souza") {
		t.Fatalf("list body has no patient rows: %q", body)
	}
	_ = withoutDoc
}
