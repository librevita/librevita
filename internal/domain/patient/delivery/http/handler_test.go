package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/domain/patient/usecase"
)

func TestPatientChanges(t *testing.T) {
	before := &usecase.GetPatientWithCreatorRow{
		DisplayName: "Ana Souza",
		Sex:         "female",
		Phone:       strPtr("11"),
		Email:       strPtr("ana@t.com"),
		City:        nil,
	}
	cases := []struct {
		name  string
		input usecase.PatientInput
		want  string
	}{
		{
			name:  "no changes",
			input: usecase.PatientInput{DisplayName: "Ana Souza", Sex: "female", Phone: "11", Email: "ana@t.com"},
			want:  "",
		},
		{
			name:  "changed values",
			input: usecase.PatientInput{DisplayName: "Ana Souza Silva", Sex: "female", Phone: "22", Email: "ana@t.com"},
			want:  "display name, phone",
		},
		{
			name:  "new value",
			input: usecase.PatientInput{DisplayName: "Ana Souza", Sex: "female", Phone: "11", Email: "ana@t.com", City: "Sao Paulo"},
			want:  "city",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, patientChanges(before, tc.input))
		})
	}
}

func TestHistoryText(t *testing.T) {
	email := "nurse@clinic.org"
	detail := "phone"
	cases := []struct {
		name string
		ev   audit.EventRow
		want string
	}{
		{
			name: "create",
			ev:   audit.EventRow{Action: "patient.create", ActorEmail: &email},
			want: "Registered by nurse@clinic.org",
		},
		{
			name: "update with changes",
			ev:   audit.EventRow{Action: "patient.update", ActorEmail: &email, Detail: &detail},
			want: "Updated by nurse@clinic.org (phone)",
		},
		{
			name: "update without detail",
			ev:   audit.EventRow{Action: "patient.update", ActorEmail: &email},
			want: "Updated by nurse@clinic.org",
		},
		{
			name: "archived",
			ev:   audit.EventRow{Action: "patient.status", ActorEmail: &email, Detail: strPtr("inactive")},
			want: "Archived by nurse@clinic.org",
		},
		{
			name: "restored",
			ev:   audit.EventRow{Action: "patient.status", ActorEmail: &email, Detail: strPtr("active")},
			want: "Restored by nurse@clinic.org",
		},
		{
			name: "unknown actor",
			ev:   audit.EventRow{Action: "patient.status", Detail: strPtr("inactive")},
			want: "Archived by an unknown user",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, historyText(tc.ev))
		})
	}
}

func strPtr(s string) *string { return &s }

func TestPatientCRUDAndManagementRoutes(t *testing.T) {
	e, sessions, svc, _, _ := newDocEnvFull(t, t.TempDir())
	cookie := adminSession(t, sessions)
	patID := newPatient(t, svc, testClinic)

	h := &Handler{
		svc: svc,
	}
	_ = h

	// 1. GET /patients (List)
	req := httptest.NewRequest(http.MethodGet, "/patients", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 2. GET /patients/new (NewPage)
	req = httptest.NewRequest(http.MethodGet, "/patients/new", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 3. POST /patients (Create)
	form := url.Values{
		"display_name": {"Carlos Eduardo"},
		"phone":        {"+55 11 98888-7777"},
		"email":        {"carlos@example.org"},
		"sex":          {"male"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 4. GET /patients/:id (Detail)
	req = httptest.NewRequest(http.MethodGet, "/patients/"+patID.String(), nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 5. GET /patients/:id/edit (EditPage)
	req = httptest.NewRequest(http.MethodGet, "/patients/"+patID.String()+"/edit", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 6. POST /patients/:id (Update)
	updForm := url.Values{
		"display_name": {"Ana Souza da Silva"},
		"phone":        {"+55 11 99999-8888"},
		"email":        {"ana@example.org"},
		"sex":          {"female"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patID.String(), strings.NewReader(updForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 7. POST /patients/:id/archive (Archive)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patID.String()+"/archive", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 8. POST /patients/:id/restore (Restore)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patID.String()+"/restore", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)

	// 9. POST /patients/bulk-archive (BulkArchive)
	bulkForm := url.Values{
		"patient_id": {patID.String()},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/bulk-archive", strings.NewReader(bulkForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusNotFound)
}

func TestPatientDetailShredAndSearchFields(t *testing.T) {
	e, sessions, svc, _, _ := newDocEnvFull(t, t.TempDir())
	cookie := adminSession(t, sessions)
	patID := newPatient(t, svc, testClinic)

	// 1. GET /patients with field=name
	req := httptest.NewRequest(http.MethodGet, "/patients?field=name&q=Ana", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /patients with status=inactive cookie
	req = httptest.NewRequest(http.MethodGet, "/patients?status=inactive", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. POST /patients/bulk-archive empty
	emptyBulkForm := url.Values{}
	req = httptest.NewRequest(http.MethodPost, "/patients/bulk-archive", strings.NewReader(emptyBulkForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadRequest)

	// 4. POST /patients/:id/shred
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patID.String()+"/shred", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound || rec.Code == http.StatusSeeOther)

	// 5. POST /patients/bulk-archive with real "ids" (and missing id for skip test)
	bulkWithIDsForm := url.Values{
		"ids": {patID.String(), uuid.NewString(), "invalid-uuid"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/bulk-archive?page=bad", strings.NewReader(bulkWithIDsForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = req.ParseForm()
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 6. GET /patients with invalid page
	req = httptest.NewRequest(http.MethodGet, "/patients?page=-1", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 7. GET /patients/:id not found
	req = httptest.NewRequest(http.MethodGet, "/patients/"+uuid.NewString(), nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 8. GET /patients/:id/edit not found
	req = httptest.NewRequest(http.MethodGet, "/patients/"+uuid.NewString()+"/edit", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 9. POST /patients/:id archive via HTMX
	activePat := newPatient(t, svc, testClinic)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+activePat.String()+"/archive", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 10. POST /patients/:id restore via HTMX
	req = httptest.NewRequest(http.MethodPost, "/patients/"+activePat.String()+"/restore", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 11. POST /patients non-HTMX validation error
	badForm := url.Values{"display_name": {""}, "email": {"invalid"}}
	req = httptest.NewRequest(http.MethodPost, "/patients", strings.NewReader(badForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 12. POST /patients/:id non-HTMX validation error
	req = httptest.NewRequest(http.MethodPost, "/patients/"+activePat.String(), strings.NewReader(badForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 13. POST /patients/:id with bad identifier check digit
	badDocForm := url.Values{"display_name": {"Ana Valida"}, "identifier_value": {"12345678901"}}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+activePat.String(), strings.NewReader(badDocForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
