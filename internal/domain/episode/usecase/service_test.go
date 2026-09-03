package usecase_test

import (
	"context"
	"librevita.org/pkg/log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	policymocks "librevita.org/internal/test/mock/core/policy"
)

type memRepo struct {
	byID     map[ident.EpisodeID]episodemodel.Episode
	patients map[ident.PatientID]bool
}

func newMemRepo() *memRepo {
	return &memRepo{byID: map[ident.EpisodeID]episodemodel.Episode{}, patients: map[ident.PatientID]bool{}}
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
func (m *memRepo) Get(_ context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID) (*episodemodel.Episode, error) {
	ep, ok := m.byID[episodeID]
	if !ok || ep.ClinicID != clinicID {
		return nil, episodemodel.ErrNotFound
	}
	cp := ep
	fillSuccessor(m.byID, &cp)
	return &cp, nil
}
func (m *memRepo) GetByPredecessor(_ context.Context, clinicID ident.ClinicID, predecessorID ident.EpisodeID) (*episodemodel.Episode, error) {
	for _, ep := range m.byID {
		if ep.ClinicID == clinicID && ep.PredecessorID != nil && *ep.PredecessorID == predecessorID {
			cp := ep
			fillSuccessor(m.byID, &cp)
			return &cp, nil
		}
	}
	return nil, episodemodel.ErrNotFound
}
func (m *memRepo) ListByPatient(_ context.Context, clinicID ident.ClinicID, patientID ident.PatientID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
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

func rejectDupPredecessor(byID map[ident.EpisodeID]episodemodel.Episode, ep episodemodel.Episode) error {
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

func fillSuccessor(byID map[ident.EpisodeID]episodemodel.Episode, ep *episodemodel.Episode) {
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
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
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
	assert.False(t, saved.ID.IsZero())

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
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
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
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
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
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
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

func TestListByPatientAndPatientGone(t *testing.T) {
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	phys := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}
	pat := &auth.Principal{ID: userID.String(), Role: auth.RolePatient, PatientID: patientID.String(), ClinicID: clinicID.String()}

	// 1. Create episode
	saved, err := svc.Create(context.Background(), phys, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
		SOAP:       episodemodel.SOAP{Subjective: "Paciente relata tosse"},
		Findings: []episodemodel.Finding{
			{
				Code:   episodemodel.Coding{Code: "tosse"},
				Value:  episodemodel.FindingValue{Kind: episodemodel.FindingValueString, String: "tosse seca"},
				Status: episodemodel.FindingStatusRecorded,
			},
		},
		Problems: []episodemodel.Problem{
			{
				Code:               episodemodel.Coding{Code: "R05"},
				Text:               "Tosse",
				ClinicalStatus:     episodemodel.ProblemClinicalActive,
				VerificationStatus: episodemodel.ProblemVerificationConfirmed,
				Category:           episodemodel.ProblemCategoryEncounter,
				Rank:               1,
			},
		},
		PlanItems: []episodemodel.PlanItem{
			{
				Kind:        episodemodel.PlanItemKindInstruction,
				Status:      episodemodel.PlanItemStatusDraft,
				Description: "Repouso e hidratação",
			},
		},
	})
	require.NoError(t, err)

	// 2. Physician lists episodes (can see drafts)
	listPhys, err := svc.ListByPatient(context.Background(), phys, clinicID, patientID)
	require.NoError(t, err)
	assert.Len(t, listPhys, 1)

	// 3. Patient lists episodes (drafts excluded)
	listPat, err := svc.ListByPatient(context.Background(), pat, clinicID, patientID)
	require.NoError(t, err)
	assert.Empty(t, listPat)

	// 4. Finalize and check patient list
	_, err = svc.Finalize(context.Background(), phys, clinicID, saved.ID)
	require.NoError(t, err)
	listPatFinal, err := svc.ListByPatient(context.Background(), pat, clinicID, patientID)
	require.NoError(t, err)
	assert.Len(t, listPatFinal, 1)

	// 5. Patient does not exist -> ErrPatientGone
	missingPatID := ident.MustParsePatient("00000000-0000-0000-0000-000000000099")
	_, err = svc.Create(context.Background(), phys, episodemodel.Episode{
		ClinicID: clinicID, PatientID: missingPatID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
	})
	assert.ErrorIs(t, err, episodemodel.ErrPatientGone)
}

func TestAmendAndSuccessorDraft(t *testing.T) {
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
	repo := newMemRepo()
	repo.patients[patientID] = true
	svc := setupEpisodeSvc(t, repo)
	phys := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}

	// 1. Create initial draft
	ep, err := svc.Create(context.Background(), phys, episodemodel.Episode{
		ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeConsultation, Class: episodemodel.CareSettingAmbulatory,
		OccurredAt: time.Now().UTC(),
		SOAP:       episodemodel.SOAP{Subjective: "Draft inicial"},
	})
	require.NoError(t, err)

	// 2. Amend on draft fails with ErrNotFinalized
	_, err = svc.Amend(context.Background(), phys, clinicID, ep.ID)
	assert.ErrorIs(t, err, episodemodel.ErrNotFinalized)

	// 3. Finalize
	finalized, err := svc.Finalize(context.Background(), phys, clinicID, ep.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusFinalized, finalized.Status)

	// Finalize again returns ErrNotDraft
	_, err = svc.Finalize(context.Background(), phys, clinicID, ep.ID)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	// UpdateDraft on finalized note fails
	_, err = svc.UpdateDraft(context.Background(), phys, *finalized)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	// 4. Amend finalized creates successor draft
	amendment1, err := svc.Amend(context.Background(), phys, clinicID, finalized.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusDraft, amendment1.Status)
	require.NotNil(t, amendment1.PredecessorID)
	assert.Equal(t, finalized.ID, *amendment1.PredecessorID)

	// 5. Amend again while amendment1 is still draft returns same draft
	amendmentRepeat, err := svc.Amend(context.Background(), phys, clinicID, finalized.ID)
	require.NoError(t, err)
	assert.Equal(t, amendment1.ID, amendmentRepeat.ID)

	// 6. Finalize amendment1
	amendmentFinalized, err := svc.Finalize(context.Background(), phys, clinicID, amendment1.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusFinalized, amendmentFinalized.Status)

	// 7. Amend original finalized note now returns ErrAlreadyAmended
	_, err = svc.Amend(context.Background(), phys, clinicID, finalized.ID)
	assert.ErrorIs(t, err, episodemodel.ErrAlreadyAmended)

	// 8. Amend of amendmentFinalized starts next link
	amendment2, err := svc.Amend(context.Background(), phys, clinicID, amendmentFinalized.ID)
	require.NoError(t, err)
	assert.Equal(t, amendmentFinalized.ID, *amendment2.PredecessorID)
}

func TestEpisodeChildItemsAndAuthCornerCases(t *testing.T) {
	clinicID := ident.MustParseClinic("00000000-0000-0000-0000-000000000001")
	userID := ident.MustParseUser("00000000-0000-0000-0000-000000000002")
	patientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000003")
	otherPatientID := ident.MustParsePatient("00000000-0000-0000-0000-000000000004")

	repo := newMemRepo()
	repo.patients[patientID] = true
	repo.patients[otherPatientID] = true
	svc := setupEpisodeSvc(t, repo)
	ctx := context.Background()

	// 1. Create with nil principal returns ErrForbidden
	_, err := svc.Create(ctx, nil, episodemodel.Episode{ClinicID: clinicID, PatientID: patientID})
	assert.ErrorIs(t, err, episodemodel.ErrForbidden)

	// 2. Create episode with findings, problems, and plan items
	phys := &auth.Principal{ID: userID.String(), Role: auth.RolePhysician, ClinicID: clinicID.String()}
	ep, err := svc.Create(ctx, phys, episodemodel.Episode{
		ClinicID:   clinicID,
		PatientID:  patientID,
		AuthorID:   userID,
		OccurredAt: time.Now().UTC(),
		SOAP:       episodemodel.SOAP{Subjective: "Dor", Objective: "Normal", Assessment: "Cefaleia", Plan: "Repouso"},
		Findings: []episodemodel.Finding{
			{
				Code:  episodemodel.Coding{System: "snomed", Code: "123", Display: "Fever"},
				Value: episodemodel.FindingValue{Kind: episodemodel.FindingValueString, String: "38.5"},
			},
		},
		Problems: []episodemodel.Problem{
			{Code: episodemodel.Coding{System: "icd10", Code: "R50", Display: "Fever"}, Text: "Febre alta"},
		},
		PlanItems: []episodemodel.PlanItem{
			{Kind: episodemodel.PlanItemKindInstruction, Description: "Beber agua"},
		},
	})
	require.NoError(t, err)
	assert.Len(t, ep.Findings, 1)
	assert.Len(t, ep.Problems, 1)
	assert.Len(t, ep.PlanItems, 1)

	// Get with nil principal returns ErrForbidden on existing ep
	_, err = svc.Get(ctx, nil, clinicID, ep.ID)
	assert.ErrorIs(t, err, episodemodel.ErrForbidden)

	// Finalize ep
	finalized, err := svc.Finalize(ctx, phys, clinicID, ep.ID)
	require.NoError(t, err)

	// 3. Patient role accessing another patient's note returns ErrForbidden
	otherPat := &auth.Principal{ID: userID.String(), Role: auth.RolePatient, PatientID: otherPatientID.String(), ClinicID: clinicID.String()}
	_, err = svc.Get(ctx, otherPat, clinicID, finalized.ID)
	assert.ErrorIs(t, err, episodemodel.ErrForbidden)

	// 4. Amend copies child items correctly
	amended, err := svc.Amend(ctx, phys, clinicID, finalized.ID)
	require.NoError(t, err)
	assert.Len(t, amended.Findings, 1)
	assert.Len(t, amended.Problems, 1)
	assert.Len(t, amended.PlanItems, 1)

	// 5. UpdateDraft on finalized episode returns ErrNotDraft
	_, err = svc.UpdateDraft(ctx, phys, *finalized)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	// 6. Finalize on finalized episode returns ErrNotDraft
	_, err = svc.Finalize(ctx, phys, clinicID, finalized.ID)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	// 7. Amend on draft episode returns ErrNotFinalized
	_, err = svc.Amend(ctx, phys, clinicID, amended.ID)
	assert.ErrorIs(t, err, episodemodel.ErrNotFinalized)
}
