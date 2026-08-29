package http_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/policy"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	episodehttp "librevita.org/internal/domain/episode/delivery/http"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	auditmocks "librevita.org/tests/mocks/core/audit"
	policymocks "librevita.org/tests/mocks/core/policy"
)

type memRepo struct {
	byID     map[uuid.UUID]episodemodel.Episode
	patients map[uuid.UUID]bool
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
func (m *memRepo) Get(_ context.Context, clinicID, episodeID uuid.UUID) (*episodemodel.Episode, error) {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return nil, episodemodel.ErrNotFound
	}
	cp := ep
	fillHTTPSuccessor(m.byID, &cp)
	return &cp, nil
}
func (m *memRepo) GetByPredecessor(_ context.Context, clinicID, predecessorID uuid.UUID) (*episodemodel.Episode, error) {
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PredecessorID != nil && *ep.PredecessorID == predecessorID {
			cp := ep
			fillHTTPSuccessor(m.byID, &cp)
			return &cp, nil
		}
	}
	return nil, episodemodel.ErrNotFound
}
func (m *memRepo) ListByPatient(_ context.Context, clinicID, patientID uuid.UUID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
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
func (m *memRepo) SetStatus(_ context.Context, clinicID, episodeID uuid.UUID, status episodemodel.EpisodeStatus) error {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return episodemodel.ErrNotFound
	}
	ep.Status = status
	m.byID[episodeID] = ep
	return nil
}
func (m *memRepo) PatientExists(_ context.Context, _, patientID uuid.UUID) (bool, error) {
	return m.patients[patientID], nil
}

func fillHTTPSuccessor(byID map[uuid.UUID]episodemodel.Episode, ep *episodemodel.Episode) {
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
	log := slog.New(slog.DiscardHandler)
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
	return episodehttp.NewHandler(usecase.NewService(repo, log, policies), nil, &clinicusecase.ClockProvider{}, &auth.CSRF{}, auditLogger)
}

func TestListFragment(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000bb")
	ep := episodemodel.Episode{
		ID:       uuid.MustParse("01990000-0000-7000-8000-0000000000aa"),
		ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	repo := &memRepo{byID: map[uuid.UUID]episodemodel.Episode{ep.ID: ep}, patients: map[uuid.UUID]bool{patientID: true}}
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
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000bb")
	aID := uuid.MustParse("01990000-0000-7000-8000-0000000000aa")
	bID := uuid.MustParse("01990000-0000-7000-8000-0000000000ab")
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
	repo := &memRepo{byID: map[uuid.UUID]episodemodel.Episode{aID: final, bID: draft}, patients: map[uuid.UUID]bool{patientID: true}}
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
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000bb")
	ep := episodemodel.Episode{
		ID:       uuid.MustParse("01990000-0000-7000-8000-0000000000aa"),
		ClinicID: clinicID, PatientID: patientID,
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusFinalized,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	repo := &memRepo{byID: map[uuid.UUID]episodemodel.Episode{ep.ID: ep}, patients: map[uuid.UUID]bool{patientID: true}}
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
	clinicID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000bb")
	authorID := uuid.MustParse("01990000-0000-7000-8000-0000000000cc")
	repo := &memRepo{byID: map[uuid.UUID]episodemodel.Episode{}, patients: map[uuid.UUID]bool{patientID: true}}
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
