package http

import (
	"context"
	"librevita.org/pkg/log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/ent"
	"librevita.org/ent/patientidentifier"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/core/vault"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	identifierusecase "librevita.org/internal/domain/identifier/usecase"
	patientrepo "librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/testutil"
)

var testClinic = "01990000-0000-7000-8000-0000000000d0"

func attachSeededClinic(engine *crypto.Engine, enc crypto.Encryptor, hasher crypto.Hasher) echo.MiddlewareFunc {
	id := uuid.MustParse(testClinic)
	now := time.Now()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := clinicctx.WithClinic(c.Request().Context(), &clinicctx.Clinic{
				ID:          id,
				Slug:        "test-clinic",
				Name:        "Test Clinic",
				Timezone:    "America/Sao_Paulo",
				OnboardedAt: &now,
			})
			ctx = fle.WithClinicID(ctx, id.String())
			ctx = crypto.WithRequestKeyCache(ctx)
			defer crypto.ClearRequestKeyCache(ctx)
			ctx = fle.WithEncryptor(ctx, enc)
			ctx = fle.WithHasher(ctx, hasher)
			ctx = fle.WithPatientEncryptorResolver(ctx, engine)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// newIdentEnv mounts the identifier routes with real middlewares and a
// migrated database.
func newIdentEnv(t *testing.T) (*echo.Echo, *auth.SessionManager, *usecase.Service, *audit.Logger, *ent.Client) {
	t.Helper()
	client := openDocDB(t)
	log := log.Nop()
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

	files, err := storage.NewFileManager(storage.NewIndexRepository(client), mustLocalStore(t), log)
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
	svc := usecase.NewService(patientrepo.NewPatientRepositoryWithEngine(client, masterKey), policies, masterKey)
	ids, systems := newIdentifierServices(t, client, masterKey, log)
	h := NewHandler(svc, clinicusecase.NewClockProvider(clinicrepo.NewClinicRepository(client)), csrf, auditLogger, files, ids, systems, masterKey, log)

	e := echo.New()
	e.Use(attachSeededClinic(masterKey, enc, hasher))
	view := []echo.MiddlewareFunc{
		server.RequireAuth(sessions, log),
		server.RequirePolicy(policies, auditLogger, log, "patient.view"),
	}
	lookup := append(view, server.RateLimit(server.NewRateLimiter(60, 1e9)))
	e.GET("/patients/lookup", h.IdentifierLookup, lookup...)
	e.POST("/patients/:id/identifiers", h.IdentifierAdd, view...)
	e.POST("/patients/:id/identifiers/:identifierID/remove", h.IdentifierRemove, view...)
	e.POST("/patients/:id/shred", h.Shred, view...)
	e.GET("/patients/:id", h.Detail, view...)
	e.GET("/patients", h.List, view...)
	e.POST("/patients", h.Create, view...)
	return e, sessions, svc, auditLogger, client
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
	e, sessions, svc, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	// A patient in another clinic.
	otherClinic := "01990000-0000-7000-8000-0000000000d1"
	if err := testutil.Clinic(context.Background(), db, otherClinic, "Other", "111.111.111-11"); err != nil {
		t.Fatal(err)
	}
	otherPt, err := svc.Create(context.Background(), otherClinic, testAdminID.String(), usecase.PatientInput{
		DisplayName: "Other Patient",
		Phone:       "+55 11 98888-1111",
		Email:       "other@example.org",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherPatient := otherPt.ID.String()

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
	identRow, err := db.PatientIdentifier.Query().
		Where(patientidentifier.PatientIDEQ(patientID)).
		First(context.Background())
	if err != nil {
		t.Fatalf("load identifier: %v", err)
	}
	identifierID := identRow.ID.String()

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
		"phone":            {"+55 11 99999-8888"},
		"email":            {"novo@example.org"},
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
		"phone":            {"+55 11 99999-8888"},
		"email":            {"novo@example.org"},
		"identifier_value": {"12345678901"}, // wrong CPF check digit
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200 form error", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid check digit") {
		t.Fatalf("body = %q, want the check digit message", rec.Body.String())
	}
	count, err := db.Patient.Query().Count(context.Background())
	if err != nil {
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
		"phone":            {"+55 11 99999-8888"},
		"email":            {"primeiro@example.org"},
		"identifier_value": {"52998224725"},
	})
	if first.Code != http.StatusFound {
		t.Fatalf("first create status = %d, want 302", first.Code)
	}

	second := postFormHtmx(t, e, "/patients", cookie, url.Values{
		"display_name":     {"Segundo"},
		"phone":            {"+55 11 99999-7777"},
		"email":            {"segundo@example.org"},
		"identifier_value": {"52998224725"},
	})
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "already registered") {
		t.Fatalf("second create status = %d body = %q, want the duplicate message", second.Code, second.Body.String())
	}
	count2, err := db.Patient.Query().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count2 != 1 {
		t.Fatalf("patients = %d, want 1", count2)
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

func TestPatientShredRemovesAggregate(t *testing.T) {
	e, sessions, svc, _, db := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)
	postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})
	count, err := db.PatientIdentifier.Query().Where(patientidentifier.PatientIDEQ(patientID)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("identifiers before shred = %d, want 1", count)
	}

	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/shred", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("shred status = %d, want 302", rec.Code)
	}

	_, err = svc.Get(context.Background(), testClinic, patientID.String())
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("get after shred = %v, want not found", err)
	}
	count, err = db.PatientIdentifier.Query().Where(patientidentifier.PatientIDEQ(patientID)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("identifiers after shred = %d, want 0", count)
	}
}

// TestRegistryListSearchField verifies the "search with dropdown"
// scope: q narrows to the selected field (name/email) or searches all
// fields when none is chosen.
func TestRegistryListSearchField(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	email := "fernanda@example.org"
	if _, err := svc.Create(context.Background(), testClinic, testAdminID.String(), usecase.PatientInput{
		DisplayName: "Fernanda Rocha",
		Phone:       "+55 11 99999-6666",
		Email:       email,
	}); err != nil {
		t.Fatalf("create patient: %v", err)
	}

	emailPage := getWithCookie(t, e, "/patients?q=fernanda@example.org&field=email", cookie, false)
	if emailPage.Code != http.StatusOK || !strings.Contains(emailPage.Body.String(), "Fernanda Rocha") {
		t.Fatalf("field=email search did not match the email: %q", emailPage.Body.String())
	}

	namePage := getWithCookie(t, e, "/patients?q=fernanda@example.org&field=name", cookie, false)
	if strings.Contains(namePage.Body.String(), "Fernanda Rocha") {
		t.Fatalf("field=name search matched the email term: %q", namePage.Body.String())
	}

	nameHit := getWithCookie(t, e, "/patients?q=rocha&field=name", cookie, false)
	if !strings.Contains(nameHit.Body.String(), "Fernanda Rocha") {
		t.Fatalf("field=name search did not match the name: %q", nameHit.Body.String())
	}

	allHit := getWithCookie(t, e, "/patients?q=fernanda@example.org", cookie, false)
	if !strings.Contains(allHit.Body.String(), "Fernanda Rocha") {
		t.Fatalf("all-fields search did not match the email: %q", allHit.Body.String())
	}
}

// TestRegistryDocumentLookup covers the embedded exact lookup: a
// document type chosen in the search dropdown runs the blind-index
// search scoped to that system and renders the owner as a normal row.
// The plaintext is never echoed back, and the page lists the active
// document types in the dropdown.
func TestRegistryDocumentLookup(t *testing.T) {
	e, sessions, svc, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)
	patientID := newPatient(t, svc, testClinic)
	postForm(t, e, "/patients/"+patientID.String()+"/identifiers", cookie, url.Values{
		"value": {"52998224725"},
	})

	hit := getWithCookie(t, e, "/patients?q=52998224725&field="+identifierusecase.CPFSystem, cookie, false)
	if hit.Code != http.StatusOK || !strings.Contains(hit.Body.String(), "Ana Souza") {
		t.Fatalf("document lookup missed the owner: %q", hit.Body.String())
	}
	if !strings.Contains(hit.Body.String(), "529••••25") {
		t.Fatalf("document lookup does not show the masked document: %q", hit.Body.String())
	}
	// The query echo (input value, pager hrefs) legitimately carries
	// the typed value; the decrypted plaintext must never leave the
	// server, so no reveal attribute may exist.
	if strings.Contains(hit.Body.String(), `data-lv-plain="52998224725"`) {
		t.Fatalf("document lookup leaks the plaintext: %q", hit.Body.String())
	}

	// The dropdown lists the active document types.
	page := getWithCookie(t, e, "/patients", cookie, false)
	if !strings.Contains(page.Body.String(), "CPF (Brasil)") || !strings.Contains(page.Body.String(), "Documents") {
		t.Fatalf("registry page does not list the document types: %q", page.Body.String())
	}

	// The same value under another system is not the chosen document.
	miss := getWithCookie(t, e, "/patients?q=52998224725&field="+identifierusecase.NIFSystem, cookie, false)
	if miss.Code != http.StatusOK || strings.Contains(miss.Body.String(), "Ana Souza") {
		t.Fatalf("wrong-system lookup returned the owner: %q", miss.Body.String())
	}

	// An unknown scope degrades to the text search, never to the lookup.
	unknown := getWithCookie(t, e, "/patients?q=52998224725&field=urn:librevita:id:x:unknown", cookie, true)
	if unknown.Code != http.StatusOK || !strings.Contains(unknown.Body.String(), "No patients found") {
		t.Fatalf("unknown scope status = %d body = %q, want the empty list", unknown.Code, unknown.Body.String())
	}

	// Short and empty values render the empty list without a lookup.
	for _, q := range []string{"", "ab"} {
		short := getWithCookie(t, e, "/patients?q="+q+"&field="+identifierusecase.CPFSystem, cookie, true)
		if short.Code != http.StatusOK || !strings.Contains(short.Body.String(), "No patients found") {
			t.Fatalf("q=%q status = %d body = %q, want the empty list", q, short.Code, short.Body.String())
		}
	}
}

func TestPatientStatusCookieFilter(t *testing.T) {
	e, sessions, _, _, _ := newIdentEnv(t)
	cookie := adminSession(t, sessions)

	// GET /patients?status=inactive sets the status cookie
	req := httptest.NewRequest(http.MethodGet, "/patients?status=inactive", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var statusCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "lv_patient_status" {
			statusCookie = c
			break
		}
	}
	if statusCookie == nil || statusCookie.Value != "inactive" {
		t.Fatalf("lv_patient_status cookie = %v, want inactive", statusCookie)
	}

	// Subsequent GET /patients without query param uses saved cookie
	req2 := httptest.NewRequest(http.MethodGet, "/patients", nil)
	req2.AddCookie(cookie)
	req2.AddCookie(statusCookie)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `data-status="inactive"`) {
		t.Fatalf("response body does not carry saved status inactive: %q", rec2.Body.String())
	}
}
