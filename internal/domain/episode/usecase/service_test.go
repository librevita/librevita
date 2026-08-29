package usecase_test

import (
	"context"
	"librevita.org/pkg/log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	policymocks "librevita.org/tests/mocks/core/policy"
)

type memRepo struct {
	byID     map[uuid.UUID]episodemodel.Episode
	patients map[uuid.UUID]bool
}

func newMemRepo() *memRepo {
	return &memRepo{byID: map[uuid.UUID]episodemodel.Episode{}, patients: map[uuid.UUID]bool{}}
}

func (m *memRepo) Create(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	if err := rejectDupPredecessor(m.byID, ep); err != nil {
		return nil, err
	}
	cp := ep
	m.byID[ep.ID] = cp
	return &cp, nil
}
func (m *memRepo) UpdateDraft(_ context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	cur, ok := m.byID[ep.ID]
	if !ok {
		return nil, episodemodel.ErrNotFound
	}
	if cur.Status != episodemodel.EpisodeStatusDraft {
		return nil, episodemodel.ErrNotDraft
	}
	m.byID[ep.ID] = ep
	return &ep, nil
}
func (m *memRepo) Get(_ context.Context, clinicID, episodeID uuid.UUID) (*episodemodel.Episode, error) {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return nil, episodemodel.ErrNotFound
	}
	cp := ep
	fillSuccessor(m.byID, &cp)
	return &cp, nil
}
func (m *memRepo) GetByPredecessor(_ context.Context, clinicID, predecessorID uuid.UUID) (*episodemodel.Episode, error) {
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PredecessorID != nil && *ep.PredecessorID == predecessorID {
			cp := ep
			fillSuccessor(m.byID, &cp)
			return &cp, nil
		}
	}
	return nil, episodemodel.ErrNotFound
}
func (m *memRepo) ListByPatient(_ context.Context, clinicID, patientID uuid.UUID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
	var out []episodemodel.Episode
	for _, ep := range m.byID {
		if ep.ClinicID != clinicID || ep.PatientID != patientID {
			continue
		}
		if status != nil && ep.Status != *status {
			continue
		}
		cp := ep
		fillSuccessor(m.byID, &cp)
		out = append(out, cp)
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

func rejectDupPredecessor(byID map[uuid.UUID]episodemodel.Episode, ep episodemodel.Episode) error {
	if ep.PredecessorID == nil {
		return nil
	}
	for _, existing := range byID {
		if existing.PredecessorID != nil && *existing.PredecessorID == *ep.PredecessorID && existing.ID != ep.ID {
			return episodemodel.ErrAlreadyAmended
		}
	}
	return nil
}

func fillSuccessor(byID map[uuid.UUID]episodemodel.Episode, ep *episodemodel.Episode) {
	for _, other := range byID {
		if other.PredecessorID != nil && *other.PredecessorID == ep.ID {
			id := other.ID
			ep.SuccessorID = &id
			return
		}
	}
}

func setupEpisodeSvc(t *testing.T, repo *memRepo) *usecase.Service {
	t.Helper()
	policyRepoMock := policymocks.NewMockRepository(t)
	policyRepoMock.EXPECT().SeedDefaults(mock.Anything, mock.Anything).Return(nil).Maybe()
	var defaultRows []policy.PolicyRow
	for name, expr := range policy.DefaultPolicies {
		defaultRows = append(defaultRows, policy.PolicyRow{Name: name, Expression: expr})
	}
	policyRepoMock.EXPECT().List(mock.Anything).Return(defaultRows, nil).Maybe()
	policies, err := policy.NewPolicyEngine(policyRepoMock, log.Nop())
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))
	return usecase.NewService(repo, policies)
}

func TestCreateGetFinalize(t *testing.T) {
	clinicID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	patientID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	p := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}

	ep := episodemodel.Episode{
		ClinicID:   clinicID,
		PatientID:  patientID,
		AuthorID:   userID,
		Type:       episodemodel.EpisodeTypeConsultation,
		Class:      episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
		SOAP:       episodemodel.SOAP{Subjective: "dor"},
	}
	saved, err := svc.Create(context.Background(), p, ep)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusDraft, saved.Status)
	assert.NotEqual(t, uuid.Nil, saved.ID)

	got, err := svc.Get(context.Background(), p, clinicID, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "dor", got.SOAP.Subjective)

	final, err := svc.Finalize(context.Background(), p, clinicID, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusFinalized, final.Status)

	_, err = svc.UpdateDraft(context.Background(), p, *final)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	amended, err := svc.Amend(context.Background(), p, clinicID, final.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusDraft, amended.Status)
	require.NotNil(t, amended.PredecessorID)
	assert.Equal(t, final.ID, *amended.PredecessorID)
	assert.NotEqual(t, final.ID, amended.ID)

	other, err := svc.Create(context.Background(), p, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	_, err = svc.Amend(context.Background(), p, clinicID, other.ID)
	assert.ErrorIs(t, err, episodemodel.ErrNotFinalized)
}

func TestAmendLinearChain(t *testing.T) {
	clinicID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	patientID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	p := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}

	first, err := svc.Create(context.Background(), p, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(), SOAP: episodemodel.SOAP{Subjective: "dor"},
	})
	require.NoError(t, err)
	final, err := svc.Finalize(context.Background(), p, clinicID, first.ID)
	require.NoError(t, err)

	b1, err := svc.Amend(context.Background(), p, clinicID, final.ID)
	require.NoError(t, err)
	b2, err := svc.Amend(context.Background(), p, clinicID, final.ID)
	require.NoError(t, err)
	assert.Equal(t, b1.ID, b2.ID)

	_, err = svc.Finalize(context.Background(), p, clinicID, b1.ID)
	require.NoError(t, err)

	_, err = svc.Amend(context.Background(), p, clinicID, final.ID)
	assert.ErrorIs(t, err, episodemodel.ErrAlreadyAmended)

	c, err := svc.Amend(context.Background(), p, clinicID, b1.ID)
	require.NoError(t, err)
	require.NotNil(t, c.PredecessorID)
	assert.Equal(t, b1.ID, *c.PredecessorID)
	assert.NotEqual(t, b1.ID, c.ID)
}

func TestPatientCannotViewDraft(t *testing.T) {
	clinicID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	patientID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	phys := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}
	saved, err := svc.Create(context.Background(), phys, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	pat := &auth.Principal{ID: userID.String(), Role: auth.RolePatient, PatientID: patientID.String(), ClinicID: clinicID.String()}
	_, err = svc.Get(context.Background(), pat, clinicID, saved.ID)
	assert.ErrorIs(t, err, episodemodel.ErrForbidden)
}

func TestReceptionistCannotWrite(t *testing.T) {
	clinicID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	patientID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	p := &auth.Principal{ID: userID.String(), Role: auth.RoleReceptionist, ClinicID: clinicID.String()}
	_, err := svc.Create(context.Background(), p, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
	})
	assert.ErrorIs(t, err, episodemodel.ErrForbidden)
}
