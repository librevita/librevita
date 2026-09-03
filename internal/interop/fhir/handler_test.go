package fhir

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	"librevita.org/internal/core/policy"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	auditmocks "librevita.org/internal/test/mock/core/audit"
	policymocks "librevita.org/internal/test/mock/core/policy"
	"librevita.org/pkg/log"
)

func TestMetadata(t *testing.T) {
	auditRepo := auditmocks.NewMockRepository(t)
	log := log.Nop()
	auditLogger, err := audit.NewLogger(auditRepo, log)
	require.NoError(t, err)
	h := NewHandler(nil, auditLogger, log)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/fhir/r4/metadata", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Metadata(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), ContentType)
	var cap CapabilityStatement
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cap))
	assert.Equal(t, "CapabilityStatement", cap.ResourceType)
	assert.Equal(t, FHIRVersion, cap.FhirVersion)
	require.Len(t, cap.Rest, 1)
	types := make([]string, 0, len(cap.Rest[0].Resource))
	for _, r := range cap.Rest[0].Resource {
		types = append(types, r.Type)
	}
	assert.Equal(t, []string{"Encounter", "Composition", "Bundle"}, types)
}

func TestCreateBundleRejectsInvalidJSON(t *testing.T) {
	auditRepo := auditmocks.NewMockRepository(t)
	log := log.Nop()
	auditLogger, err := audit.NewLogger(auditRepo, log)
	require.NoError(t, err)
	h := NewHandler(nil, auditLogger, log)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, ContentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	c.Set("server.principal", &auth.Principal{ID: clinicID.String(), Role: auth.RolePhysician})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.CreateBundle(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var oo OperationOutcome
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &oo))
	assert.Equal(t, "OperationOutcome", oo.ResourceType)
}

func TestFHIRErrorHidesInternalError(t *testing.T) {
	auditRepo := auditmocks.NewMockRepository(t)
	auditLogger, err := audit.NewLogger(auditRepo, log.Nop())
	require.NoError(t, err)
	rec := log.NewRecorder()
	h := NewHandler(nil, auditLogger, rec)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/fhir/r4/Encounter/x", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(req, w)
	require.NoError(t, h.fhirError(c, errors.New("secret internals")))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "secret internals")
	assert.Contains(t, w.Body.String(), `"diagnostics":"internal error"`)
	assert.Contains(t, rec.Messages(), "fhir internal error")
}

type memEpisodeRepo struct {
	byID map[ident.EpisodeID]episodemodel.Episode
}

func (m *memEpisodeRepo) Create(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
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
func (m *memEpisodeRepo) UpdateDraft(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	m.byID[ep.ID] = ep
	return &ep, nil
}
func (m *memEpisodeRepo) Get(_ context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID) (*episodemodel.Episode, error) {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return nil, episodemodel.ErrNotFound
	}
	cp := ep
	fillFHIRSuccessor(m.byID, &cp)
	return &cp, nil
}
func (m *memEpisodeRepo) GetByPredecessor(_ context.Context, clinicID ident.ClinicID, predecessorID ident.EpisodeID) (*episodemodel.Episode, error) {
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PredecessorID != nil && *ep.PredecessorID == predecessorID {
			cp := ep
			fillFHIRSuccessor(m.byID, &cp)
			return &cp, nil
		}
	}
	return nil, episodemodel.ErrNotFound
}
func (m *memEpisodeRepo) ListByPatient(_ context.Context, clinicID ident.ClinicID, patientID ident.PatientID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
	var out []episodemodel.Episode
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PatientID == patientID {
			if status != nil && ep.Status != *status {
				continue
			}
			cp := ep
			fillFHIRSuccessor(m.byID, &cp)
			out = append(out, cp)
		}
	}
	return out, nil
}
func (m *memEpisodeRepo) SetStatus(_ context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID, status episodemodel.EpisodeStatus) error {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return episodemodel.ErrNotFound
	}
	ep.Status = status
	m.byID[episodeID] = ep
	return nil
}
func (m *memEpisodeRepo) PatientExists(_ context.Context, _ ident.ClinicID, _ ident.PatientID) (bool, error) {
	return true, nil
}

func fillFHIRSuccessor(byID map[ident.EpisodeID]episodemodel.Episode, ep *episodemodel.Episode) {
	for _, other := range byID {
		if other.PredecessorID != nil && *other.PredecessorID == ep.ID {
			id := other.ID
			ep.SuccessorID = &id
			return
		}
	}
}

func viewTestHandler(t *testing.T, auditRepo *auditmocks.MockRepository, ep episodemodel.Episode) (*Handler, ident.ClinicID) {
	t.Helper()
	log := log.Nop()
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
	repo := &memEpisodeRepo{byID: map[ident.EpisodeID]episodemodel.Episode{ep.ID: ep}}
	return NewHandler(usecase.NewService(repo, policies), auditLogger, log), ep.ClinicID
}

func sampleEpisode() episodemodel.Episode {
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	return episodemodel.Episode{
		ID:         ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa"),
		ClinicID:   clinicID,
		PatientID:  ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb"),
		AuthorID:   ident.MustParseUser("01990000-0000-7000-8000-0000000000cc"),
		Type:       episodemodel.EpisodeTypeConsultation,
		Status:     episodemodel.EpisodeStatusFinalized,
		Class:      episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC),
		SOAP:       episodemodel.SOAP{Subjective: "dor"},
	}
}

func expectChartView(auditRepo *auditmocks.MockRepository, resource string) {
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Once()
	auditRepo.EXPECT().Record(mock.Anything, mock.MatchedBy(func(ev audit.Event) bool {
		return ev.Action == "chart.view" && ev.Resource == resource && ev.Result == audit.AuditResultSuccess
	}), mock.Anything, mock.Anything).Return(nil).Once()
}

func physicianContext(e *echo.Echo, method, path string, clinicID ident.ClinicID) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:   uuid.MustParse("01990000-0000-7000-8000-0000000000cc").String(),
		Role: auth.RolePhysician, Name: "Dr",
	})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	return c, rec
}

func TestCreateBundleCreateReturns201(t *testing.T) {
	ep := sampleEpisode()
	ep.Status = episodemodel.EpisodeStatusDraft
	ep.ID = ident.MustParseEpisode("01990000-0000-7000-8000-0000000000ab")
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Once()
	auditRepo.EXPECT().Record(mock.Anything, mock.MatchedBy(func(ev audit.Event) bool {
		return ev.Action == "chart.create"
	}), mock.Anything, mock.Anything).Return(nil).Once()
	h, clinicID := viewTestHandler(t, auditRepo, sampleEpisode())

	bundle, err := ToDocumentBundle(ep, DocumentContext{})
	require.NoError(t, err)
	raw, err := json.Marshal(bundle)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", strings.NewReader(string(raw)))
	req.Header.Set(echo.HeaderContentType, ContentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:   uuid.MustParse("01990000-0000-7000-8000-0000000000cc").String(),
		Role: auth.RolePhysician, Name: "Dr",
	})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.CreateBundle(c))
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/fhir/r4/Composition/"+ep.ID.String()+"/$document")
}

func TestCreateBundleUpdateReturns200(t *testing.T) {
	ep := sampleEpisode()
	ep.Status = episodemodel.EpisodeStatusDraft
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Once()
	auditRepo.EXPECT().Record(mock.Anything, mock.MatchedBy(func(ev audit.Event) bool {
		return ev.Action == "chart.update"
	}), mock.Anything, mock.Anything).Return(nil).Once()
	h, clinicID := viewTestHandler(t, auditRepo, ep)

	bundle, err := ToDocumentBundle(ep, DocumentContext{})
	require.NoError(t, err)
	raw, err := json.Marshal(bundle)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", strings.NewReader(string(raw)))
	req.Header.Set(echo.HeaderContentType, ContentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID: ep.AuthorID.String(), Role: auth.RolePhysician, Name: "Dr",
	})
	c.SetRequest(req.WithContext(clinicctx.WithClinic(req.Context(), &clinicctx.Clinic{ID: clinicID, Slug: "t", Name: "T"})))
	require.NoError(t, h.CreateBundle(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDocumentRecordsChartView(t *testing.T) {
	ep := sampleEpisode()
	auditRepo := auditmocks.NewMockRepository(t)
	expectChartView(auditRepo, "episode:"+ep.ID.String())
	h, clinicID := viewTestHandler(t, auditRepo, ep)

	e := echo.New()
	c, rec := physicianContext(e, http.MethodGet, "/fhir/r4/Composition/"+ep.ID.String()+"/$document", clinicID)
	c.SetParamNames("id")
	c.SetParamValues(ep.ID.String())
	require.NoError(t, h.Document(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGetEncounterRecordsChartView(t *testing.T) {
	ep := sampleEpisode()
	auditRepo := auditmocks.NewMockRepository(t)
	expectChartView(auditRepo, "episode:"+ep.ID.String())
	h, clinicID := viewTestHandler(t, auditRepo, ep)

	e := echo.New()
	c, rec := physicianContext(e, http.MethodGet, "/fhir/r4/Encounter/"+ep.ID.String(), clinicID)
	c.SetParamNames("id")
	c.SetParamValues(ep.ID.String())
	require.NoError(t, h.GetEncounter(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSearchEncounterRecordsChartView(t *testing.T) {
	ep := sampleEpisode()
	auditRepo := auditmocks.NewMockRepository(t)
	expectChartView(auditRepo, "patient:"+ep.PatientID.String())
	h, clinicID := viewTestHandler(t, auditRepo, ep)

	e := echo.New()
	c, rec := physicianContext(e, http.MethodGet, "/fhir/r4/Encounter?patient="+ep.PatientID.String(), clinicID)
	require.NoError(t, h.SearchEncounter(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDocumentDoesNotAuditMissingEpisode(t *testing.T) {
	ep := sampleEpisode()
	auditRepo := auditmocks.NewMockRepository(t)
	h, clinicID := viewTestHandler(t, auditRepo, ep)

	e := echo.New()
	missing := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000ff")
	c, rec := physicianContext(e, http.MethodGet, "/fhir/r4/Composition/"+missing.String()+"/$document", clinicID)
	c.SetParamNames("id")
	c.SetParamValues(missing.String())
	require.NoError(t, h.Document(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFHIREndpointsUnauthenticatedAndErrors(t *testing.T) {
	ep := sampleEpisode()
	auditRepo := auditmocks.NewMockRepository(t)
	h, clinicID := viewTestHandler(t, auditRepo, ep)
	e := echo.New()

	// 1. Unauthenticated CreateBundle
	req := httptest.NewRequest(http.MethodPost, "/fhir/r4/Bundle", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.CreateBundle(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 2. Unauthenticated Document
	req = httptest.NewRequest(http.MethodGet, "/fhir/r4/Composition/"+ep.ID.String()+"/$document", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	require.NoError(t, h.Document(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 3. Unauthenticated GetEncounter
	req = httptest.NewRequest(http.MethodGet, "/fhir/r4/Encounter/"+ep.ID.String(), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	require.NoError(t, h.GetEncounter(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 4. Unauthenticated SearchEncounter
	req = httptest.NewRequest(http.MethodGet, "/fhir/r4/Encounter?patient="+ep.PatientID.String(), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	require.NoError(t, h.SearchEncounter(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 5. SearchEncounter missing patient query
	cAuth, recAuth := physicianContext(e, http.MethodGet, "/fhir/r4/Encounter", clinicID)
	require.NoError(t, h.SearchEncounter(cAuth))
	assert.Equal(t, http.StatusBadRequest, recAuth.Code)

	// 6. Invalid episode ID in Document
	cAuth2, recAuth2 := physicianContext(e, http.MethodGet, "/fhir/r4/Composition/invalid-uuid/$document", clinicID)
	cAuth2.SetParamNames("id")
	cAuth2.SetParamValues("invalid-uuid")
	require.NoError(t, h.Document(cAuth2))
	assert.Equal(t, http.StatusBadRequest, recAuth2.Code)

	// 7. Invalid encounter ID in GetEncounter
	cAuth3, recAuth3 := physicianContext(e, http.MethodGet, "/fhir/r4/Encounter/invalid-uuid", clinicID)
	cAuth3.SetParamNames("id")
	cAuth3.SetParamValues("invalid-uuid")
	require.NoError(t, h.GetEncounter(cAuth3))
	assert.Equal(t, http.StatusBadRequest, recAuth3.Code)

	// 8. SearchEncounter invalid patient UUID
	cAuth4, recAuth4 := physicianContext(e, http.MethodGet, "/fhir/r4/Encounter?patient=invalid-uuid", clinicID)
	require.NoError(t, h.SearchEncounter(cAuth4))
	assert.Equal(t, http.StatusBadRequest, recAuth4.Code)

	// 9. Check all fhirError error mappings
	for _, tc := range []struct {
		err  error
		code int
	}{
		{episodemodel.ErrForbidden, http.StatusForbidden},
		{episodemodel.ErrNotDraft, http.StatusConflict},
		{episodemodel.ErrNotFinalized, http.StatusConflict},
		{episodemodel.ErrAlreadyAmended, http.StatusConflict},
		{episodemodel.ErrInvalidSOAP, http.StatusBadRequest},
		{episodemodel.ErrPatientGone, http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		c := e.NewContext(req, w)
		require.NoError(t, h.fhirError(c, tc.err))
		assert.Equal(t, tc.code, w.Code)
	}
}
