package http_test

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

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/keystore"
	"librevita.org/internal/core/policy"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	episodehttp "librevita.org/internal/domain/episode/delivery/http"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	patientmodel "librevita.org/internal/domain/patient/model"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	auditmocks "librevita.org/tests/mocks/core/audit"
	policymocks "librevita.org/tests/mocks/core/policy"
	modelmocks "librevita.org/tests/mocks/domain/patient/model"
)

type memRepo struct {
	byID     map[ident.EpisodeID]episodemodel.Episode
	patients map[ident.PatientID]bool
}

func (m *memRepo) Create(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	if ep.PredecessorID != nil {
		for _, existing := range m.byID {
			if existing.PredecessorID != nil && *existing.PredecessorID == *ep.PredecessorID && existing.ID != ep.ID {
				return nil, episodemodel.ErrAlreadyAmended
			}
		}
	}
	m.byID[ep.ID] = ep
	return &ep, nil
}
func (m *memRepo) UpdateDraft(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	m.byID[ep.ID] = ep
	return &ep, nil
}
func (m *memRepo) Get(_ context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID) (*episodemodel.Episode, error) {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return nil, episodemodel.ErrNotFound
	}
	cp := ep
	fillHTTPSuccessor(m.byID, &cp)
	return &cp, nil
}
func (m *memRepo) GetByPredecessor(_ context.Context, clinicID ident.ClinicID, predecessorID ident.EpisodeID) (*episodemodel.Episode, error) {
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PredecessorID != nil && *ep.PredecessorID == predecessorID {
			cp := ep
			fillHTTPSuccessor(m.byID, &cp)
			return &cp, nil
		}
	}
	return nil, episodemodel.ErrNotFound
}
func (m *memRepo) ListByPatient(_ context.Context, clinicID ident.ClinicID, patientID ident.PatientID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
	var out []episodemodel.Episode
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PatientID == patientID {
			if status != nil && ep.Status != *status {
				continue
			}
			cp := ep
			fillHTTPSuccessor(m.byID, &cp)
			out = append(out, cp)
		}
	}
	return out, nil
}
func (m *memRepo) SetStatus(_ context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID, status episodemodel.EpisodeStatus) error {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return episodemodel.ErrNotFound
	}
	ep.Status = status
	m.byID[episodeID] = ep
	return nil
}
func (m *memRepo) PatientExists(_ context.Context, _ ident.ClinicID, patientID ident.PatientID) (bool, error) {
	return m.patients[patientID], nil
}

func fillHTTPSuccessor(byID map[ident.EpisodeID]episodemodel.Episode, ep *episodemodel.Episode) {
	for _, other := range byID {
		if other.PredecessorID != nil && *other.PredecessorID == ep.ID {
			id := other.ID
			ep.SuccessorID = &id
			return
		}
	}
}

func setupHandler(t *testing.T, repo *memRepo, auditRepo *auditmocks.MockRepository) *episodehttp.Handler {
	t.Helper()
	log := log.Nop()
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditLogger, err := audit.NewLogger(auditRepo, log)
	require.NoError(t, err)
	policyRepo := policymocks.NewMockRepository(t)
	policyRepo.EXPECT().SeedDefaults(mock.Anything, mock.Anything).Return(nil).Maybe()
	var rows []policy.PolicyRow
	for name, expr := range policy.DefaultPolicies {
		rows = append(rows, policy.PolicyRow{Name: name, Expression: expr})
	}
	policyRepo.EXPECT().List(mock.Anything).Return(rows, nil).Maybe()
	policies, err := policy.NewPolicyEngine(policyRepo, log)
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	patRepo := modelmocks.NewMockPatientRepository(t)
	patRepo.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(&patientmodel.Patient{
		DisplayName: "Ana Souza",
	}, nil).Maybe()
	patRepo.EXPECT().GetWithCreator(mock.Anything, mock.Anything, mock.Anything).Return(&patientmodel.GetPatientWithCreatorRow{
		DisplayName: "Ana Souza",
	}, nil).Maybe()
	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	masterKey, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	require.NoError(t, err)
	patSvc := patientusecase.NewService(patRepo, policies, masterKey)

	return episodehttp.NewHandler(usecase.NewService(repo, policies), patSvc, &clinicusecase.ClockProvider{}, &auth.CSRF{}, auditLogger)
}

func TestListFragment(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	ep := episodemodel.Episode{
		ID:       ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa"),
		ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{ep.ID: ep}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "New note")
	assert.Contains(t, rec.Body.String(), "Consultation")
	assert.NotContains(t, rec.Body.String(), "Amend")
}

func TestListHidesAmendWhenSuccessorExists(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	aID := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa")
	bID := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000ab")
	pred := aID
	final := episodemodel.Episode{
		ID: aID, ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusFinalized,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	draft := episodemodel.Episode{
		ID: bID, ClinicID: clinicID, PatientID: patientID, PredecessorID: &pred,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{aID: final, bID: draft}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Amend")
	assert.Contains(t, rec.Body.String(), "/patients/"+patientID.String()+"/episodes/"+bID.String()+"/edit")
}

func TestListShowsAmendWhenFinalized(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	ep := episodemodel.Episode{
		ID:       ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa"),
		ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusFinalized,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{ep.ID: ep}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.List(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Amend")
}

func TestCreateDraft(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-0000000000cc")
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Once()
	auditRepo.EXPECT().Record(mock.Anything, mock.MatchedBy(func(ev audit.Event) bool {
		return ev.Action == "chart.create"
	}), mock.Anything, mock.Anything).Return(nil).Once()
	h := setupHandler(t, repo, auditRepo)

	form := url.Values{
		"episode_type": {"consultation"},
		"class":        {"ambulatory"},
		"occurred_at":  {"2026-08-28T15:00"},
		"subjective":   {"dor"},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Create(c))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Len(t, repo.byID, 1)
}

func TestEpisodeNewPageAndEditPage(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	epID := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa")
	ep := episodemodel.Episode{
		ID: epID, ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Queixa principal"},
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{epID: ep}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()

	// 1. GET /patients/:id/episodes/new (NewPage)
	req := httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes/new", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.NewPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /patients/:id/episodes/:episodeID/edit (EditPage)
	req = httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes/"+epID.String()+"/edit", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.EditPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. GET /patients/:id/episodes/:episodeID (View)
	req = httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes/"+epID.String(), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: uuid.New().String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.View(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEpisodeUpdateFinalizeAmend(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-0000000000cc")
	epID := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa")
	ep := episodemodel.Episode{
		ID: epID, ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Queixa inicial"},
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{epID: ep}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()

	// 1. POST /patients/:id/episodes/:episodeID (Update)
	updateForm := url.Values{
		"episode_type": {"consultation"},
		"class":        {"ambulatory"},
		"occurred_at":  {"2026-08-28T15:00"},
		"subjective":   {"Queixa atualizada"},
	}
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String(), strings.NewReader(updateForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Update(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 2. Dynamic form actions (add finding)
	formWithAdd := url.Values{
		"episode_type": {"consultation"},
		"add":          {"finding"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String(), strings.NewReader(formWithAdd.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Update(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. POST /patients/:id/episodes/:episodeID/finalize (Finalize)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String()+"/finalize", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Finalize(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 4. POST /patients/:id/episodes/:episodeID/amend (Amend)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String()+"/amend", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Amend(c))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestEpisodeHTTPValidationAndErrors(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-000000000002")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-000000000003")
	epID := ident.MustParseEpisode("01990000-0000-7000-8000-000000000010")

	epFinalized := episodemodel.Episode{
		ID: epID, ClinicID: clinicID, PatientID: patientID, AuthorID: authorID,
		Status: episodemodel.EpisodeStatusFinalized, Type: episodemodel.EpisodeTypeConsultation,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Pronto"},
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{epID: epFinalized}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()

	// 1. EditPage when already finalized redirects to View
	req := httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes/"+epID.String()+"/edit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.EditPage(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 2. List without HTMX redirects to /patients/:id
	req = httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.List(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 3. Create with non-existent patient returns 400 Bad Request
	missingPatientID := ident.MustParsePatient("01990000-0000-7000-8000-000000000099")
	badForm := url.Values{
		"episode_type": {"consultation"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+missingPatientID.String()+"/episodes", strings.NewReader(badForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(missingPatientID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Create(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 4. Create with add=finding (re-renders form with added item)
	addFindingForm := url.Values{
		"add":          {"finding"},
		"episode_type": {"consultation"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes", strings.NewReader(addFindingForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Create(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. Create with finalize=1
	finalizeForm := url.Values{
		"finalize":     {"1"},
		"episode_type": {"consultation"},
		"care_setting": {"ambulatory"},
		"subjective":   {"Full Note Subjective"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes", strings.NewReader(finalizeForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(patientID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Create(c))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestEpisodeUpdateAddItemsAndAmendEdgeCases(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-000000000002")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-000000000003")
	epID := ident.MustParseEpisode("01990000-0000-7000-8000-000000000020")

	epDraft := episodemodel.Episode{
		ID: epID, ClinicID: clinicID, PatientID: patientID, AuthorID: authorID,
		Status: episodemodel.EpisodeStatusDraft, Type: episodemodel.EpisodeTypeConsultation,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Draft inicial"},
	}
	repo := &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{epID: epDraft}, patients: map[ident.PatientID]bool{patientID: true}}
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	h := setupHandler(t, repo, auditRepo)

	e := echo.New()

	// 1. Update with add=problem
	addProblemForm := url.Values{
		"add":          {"problem"},
		"episode_type": {"consultation"},
	}
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String(), strings.NewReader(addProblemForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Update(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. Update with add=plan_item
	addPlanForm := url.Values{
		"add":          {"plan_item"},
		"episode_type": {"consultation"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+epID.String(), strings.NewReader(addPlanForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Update(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. ViewPage on episode with findings and problems
	epWithItems := epDraft
	epWithItems.Findings = []episodemodel.Finding{
		{Code: episodemodel.Coding{Code: "F1", Display: "Febre"}, Value: episodemodel.FindingValue{Kind: episodemodel.FindingValueString, String: "38.5"}},
	}
	epWithItems.Problems = []episodemodel.Problem{
		{Code: episodemodel.Coding{Code: "P1", Display: "Gripe"}, Text: "Gripe Forte"},
	}
	epWithItems.PlanItems = []episodemodel.PlanItem{
		{Kind: episodemodel.PlanItemKindInstruction, Description: "Tomar remedio"},
	}
	repo.byID[epID] = epWithItems

	req = httptest.NewRequest(http.MethodGet, "/patients/"+patientID.String()+"/episodes/"+epID.String(), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), epID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.View(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEpisodeFinalizeAndAmendFlow(t *testing.T) {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-000000000002")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-000000000003")
	finalID := ident.MustParseEpisode("01990000-0000-7000-8000-000000000030")

	finalEp := episodemodel.Episode{
		ID: finalID, ClinicID: clinicID, PatientID: patientID, AuthorID: authorID,
		Status: episodemodel.EpisodeStatusFinalized, Type: episodemodel.EpisodeTypeConsultation,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Finalizado", Objective: "O", Assessment: "A", Plan: "P"},
	}

	draftID := ident.MustParseEpisode("01990000-0000-7000-8000-000000000031")
	draftEp := episodemodel.Episode{
		ID: draftID, ClinicID: clinicID, PatientID: patientID, AuthorID: authorID,
		Status: episodemodel.EpisodeStatusDraft, Type: episodemodel.EpisodeTypeConsultation,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
		SOAP: episodemodel.SOAP{Subjective: "Rascunho", Objective: "O", Assessment: "A", Plan: "P"},
	}

	repo := &memRepo{
		byID:     map[ident.EpisodeID]episodemodel.Episode{finalID: finalEp, draftID: draftEp},
		patients: map[ident.PatientID]bool{patientID: true},
	}
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	h := setupHandler(t, repo, auditRepo)
	e := echo.New()

	// 1. Amend finalized episode -> redirects to edit page
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+finalID.String()+"/amend", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), finalID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Amend(c))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/edit")

	// 2. Finalize already finalized episode returns conflict (httpError mapping)
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+finalID.String()+"/finalize", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), finalID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	err := h.Finalize(c)
	assert.Error(t, err)

	// 3. Update with finalize=1
	form := url.Values{
		"finalize":   {"1"},
		"type":       {"consultation"},
		"class":      {"ambulatory"},
		"subjective": {"Dor de cabeca"},
		"objective":  {"PA 120x80"},
		"assessment": {"Cefaleia"},
		"plan":       {"Repouso"},
	}
	req = httptest.NewRequest(http.MethodPost, "/patients/"+patientID.String()+"/episodes/"+draftID.String(), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "episodeID")
	c.SetParamValues(patientID.String(), draftID.String())
	c.Set("server.principal", &auth.Principal{ID: authorID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.Update(c))
	assert.Equal(t, http.StatusFound, rec.Code)
}
