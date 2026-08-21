package usecase_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/internal/domain/patient/usecase"
	policymocks "librevita.org/tests/mocks/core/policy"
	patientmocks "librevita.org/tests/mocks/domain/patient/model"
)

var (
	testClinicID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	testUserID   = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	missingID    = uuid.MustParse("00000000-0000-0000-0000-00000000ffff")
	ghostID      = uuid.MustParse("00000000-0000-0000-0000-00000000fffe")
)

func uuidStrPtrTest(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func validInput() usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: "Maria Oliveira",
		BirthDate:   "1985-03-14",
		Sex:         patientmodel.SexFemale,
		Phone:       "+55 11 99999-0000",
		Email:       "maria@example.org",
		City:        "São Paulo",
		State:       "SP",
	}
}

func setupPatientTest(t *testing.T) (
	*patientmocks.MockPatientRepository,
	*usecase.Service,
) {
	t.Helper()

	policyRepoMock := policymocks.NewMockRepository(t)
	policyRepoMock.EXPECT().SeedDefaults(mock.Anything, mock.Anything).Return(nil).Maybe()
	var defaultRows []policy.PolicyRow
	for name, expr := range policy.DefaultPolicies {
		defaultRows = append(defaultRows, policy.PolicyRow{
			Name:       name,
			Expression: expr,
		})
	}
	policyRepoMock.EXPECT().List(mock.Anything).Return(defaultRows, nil).Maybe()

	policies, err := policy.NewPolicyEngine(policyRepoMock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	repoMock := patientmocks.NewMockPatientRepository(t)
	svc := usecase.NewService(repoMock, slog.New(slog.DiscardHandler), policies)

	return repoMock, svc
}

func TestCreateAndGet(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var savedPatient patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p patientmodel.Patient) bool {
		return p.ClinicID == testClinicID && p.DisplayName == "Maria Oliveira"
	})).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	pt, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), validInput())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, pt.ID)
	assert.Equal(t, patientmodel.PatientStatusActive, pt.Status)

	repoMock.EXPECT().Get(mock.Anything, testClinicID, pt.ID).Return(&savedPatient, nil).Once()

	got, err := svc.Get(context.Background(), testClinicID.String(), pt.ID.String())
	require.NoError(t, err)
	assert.Equal(t, "Maria Oliveira", got.DisplayName)
}

func TestCreateValidation(t *testing.T) {
	_, svc := setupPatientTest(t)

	cases := []struct {
		name   string
		mutate func(*usecase.PatientInput)
	}{
		{"missing name", func(in *usecase.PatientInput) { in.DisplayName = " " }},
		{"bad sex", func(in *usecase.PatientInput) { in.Sex = patientmodel.Sex("alien") }},
		{"bad birth date", func(in *usecase.PatientInput) { in.BirthDate = "14/03/1985" }},
		{"bad email", func(in *usecase.PatientInput) { in.Email = "not-an-email" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			_, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), in)
			require.Error(t, err)
			var v *usecase.ValidationError
			require.ErrorAs(t, err, &v)
		})
	}
}

func TestUpdate(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var savedPatient patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	pt, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), validInput())
	require.NoError(t, err)

	in := validInput()
	in.DisplayName = "Maria O. Lima"

	repoMock.EXPECT().Get(mock.Anything, testClinicID, pt.ID).Return(&savedPatient, nil).Once()
	repoMock.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p patientmodel.Patient) bool {
		return p.ID == pt.ID && p.ClinicID == testClinicID && p.DisplayName == "Maria O. Lima"
	})).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.UpdatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	updated, err := svc.Update(context.Background(), testClinicID.String(), pt.ID.String(), in)
	require.NoError(t, err)
	assert.Equal(t, "Maria O. Lima", updated.DisplayName)

	// Update missing patient
	repoMock.EXPECT().Get(mock.Anything, testClinicID, missingID).Return(nil, usecase.ErrNotFound).Once()
	_, err = svc.Update(context.Background(), testClinicID.String(), missingID.String(), in)
	require.ErrorIs(t, err, usecase.ErrNotFound)
}

func TestListAndSearch(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var patients []patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
		patients = append(patients, p)
		return &p, nil
	}).Times(3)

	for _, name := range []string{"Ana Souza", "Bruno Lima", "Carla Dias"} {
		in := validInput()
		in.DisplayName = name
		_, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), in)
		require.NoError(t, err)
	}

	repoMock.EXPECT().ListByClinicAndStatus(mock.Anything, testClinicID, (*patientmodel.PatientStatus)(nil)).Return(patients, nil).Times(3)

	all, total, err := svc.List(context.Background(), testClinicID.String(), "", "", "", 50, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, all, 3)

	hit, _, err := svc.List(context.Background(), testClinicID.String(), "bruno", "", "", 50, 0)
	require.NoError(t, err)
	require.Len(t, hit, 1)
	assert.Equal(t, "Bruno Lima", hit[0].DisplayName)

	none, _, err := svc.List(context.Background(), testClinicID.String(), "zzz", "", "", 50, 0)
	require.NoError(t, err)
	assert.Empty(t, none)

	// Status filter
	repoMock.EXPECT().BulkSetStatus(mock.Anything, testClinicID, []uuid.UUID{hit[0].ID}, patientmodel.PatientStatusInactive).Return(1, nil).Once()
	err = svc.SetStatus(context.Background(), testClinicID.String(), hit[0].ID.String(), patientmodel.PatientStatusInactive)
	require.NoError(t, err)

	// Update record in mock slice to inactive
	activeStatus := patientmodel.PatientStatusActive
	inactiveStatus := patientmodel.PatientStatusInactive
	patients[1].Status = inactiveStatus

	repoMock.EXPECT().ListByClinicAndStatus(mock.Anything, testClinicID, &activeStatus).Return([]patientmodel.Patient{patients[0], patients[2]}, nil).Once()
	active, _, err := svc.List(context.Background(), testClinicID.String(), "", patientmodel.PatientStatusActive.String(), "", 50, 0)
	require.NoError(t, err)
	assert.Len(t, active, 2)

	repoMock.EXPECT().ListByClinicAndStatus(mock.Anything, testClinicID, &inactiveStatus).Return([]patientmodel.Patient{patients[1]}, nil).Once()
	inactive, _, err := svc.List(context.Background(), testClinicID.String(), "", patientmodel.PatientStatusInactive.String(), "", 50, 0)
	require.NoError(t, err)
	assert.Len(t, inactive, 1)
}

func TestSetStatus(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var savedPatient patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	pt, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), validInput())
	require.NoError(t, err)

	repoMock.EXPECT().BulkSetStatus(mock.Anything, testClinicID, []uuid.UUID{pt.ID}, patientmodel.PatientStatusInactive).Return(1, nil).Once()
	err = svc.SetStatus(context.Background(), testClinicID.String(), pt.ID.String(), patientmodel.PatientStatusInactive)
	require.NoError(t, err)

	savedPatient.Status = patientmodel.PatientStatusInactive
	repoMock.EXPECT().Get(mock.Anything, testClinicID, pt.ID).Return(&savedPatient, nil).Once()

	got, err := svc.Get(context.Background(), testClinicID.String(), pt.ID.String())
	require.NoError(t, err)
	assert.Equal(t, patientmodel.PatientStatusInactive, got.Status)
}

func TestCount(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	repoMock.EXPECT().Count(mock.Anything, testClinicID).Return(0, nil).Once()
	n, err := svc.Count(context.Background(), testClinicID.String())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	repoMock.EXPECT().Create(mock.Anything, mock.Anything).Return(&patientmodel.Patient{
		ID:       uuid.New(),
		ClinicID: testClinicID,
	}, nil).Once()

	_, err = svc.Create(context.Background(), testClinicID.String(), testUserID.String(), validInput())
	require.NoError(t, err)

	repoMock.EXPECT().Count(mock.Anything, testClinicID).Return(1, nil).Once()
	n, err = svc.Count(context.Background(), testClinicID.String())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestCreateRecordsRegistrar(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var savedPatient patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p patientmodel.Patient) bool {
		return p.CreatedBy != nil && *p.CreatedBy == testUserID
	})).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	pt, err := svc.Create(context.Background(), testClinicID.String(), testUserID.String(), validInput())
	require.NoError(t, err)

	creatorEmail := "ana@example.org"
	creatorName := "Ana"
	repoMock.EXPECT().GetWithCreator(mock.Anything, testClinicID, pt.ID).Return(&patientmodel.GetPatientWithCreatorRow{
		ID:           savedPatient.ID,
		ClinicID:     savedPatient.ClinicID,
		DisplayName:  savedPatient.DisplayName,
		Status:       savedPatient.Status,
		CreatedBy:    savedPatient.CreatedBy,
		CreatedAt:    savedPatient.CreatedAt,
		UpdatedAt:    savedPatient.UpdatedAt,
		CreatorEmail: &creatorEmail,
		CreatorName:  &creatorName,
	}, nil).Once()

	row, err := svc.GetWithCreator(context.Background(), testClinicID.String(), pt.ID.String())
	require.NoError(t, err)
	require.NotNil(t, row.CreatedBy)
	assert.Equal(t, testUserID.String(), row.CreatedBy.String())
	assert.Equal(t, "ana@example.org", orEmpty(row.CreatorEmail))
}

func TestGetWithCreatorMissingUser(t *testing.T) {
	repoMock, svc := setupPatientTest(t)

	var savedPatient patientmodel.Patient
	repoMock.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p patientmodel.Patient) bool {
		return p.CreatedBy != nil && *p.CreatedBy == ghostID
	})).RunAndReturn(func(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
		p.CreatedAt = time.Now()
		savedPatient = p
		return &p, nil
	}).Once()

	pt, err := svc.Create(context.Background(), testClinicID.String(), ghostID.String(), validInput())
	require.NoError(t, err)

	repoMock.EXPECT().GetWithCreator(mock.Anything, testClinicID, pt.ID).Return(&patientmodel.GetPatientWithCreatorRow{
		ID:           savedPatient.ID,
		ClinicID:     savedPatient.ClinicID,
		DisplayName:  savedPatient.DisplayName,
		Status:       savedPatient.Status,
		CreatedBy:    savedPatient.CreatedBy,
		CreatedAt:    savedPatient.CreatedAt,
		UpdatedAt:    savedPatient.UpdatedAt,
		CreatorEmail: nil,
		CreatorName:  nil,
	}, nil).Once()

	row, err := svc.GetWithCreator(context.Background(), testClinicID.String(), pt.ID.String())
	require.NoError(t, err)
	assert.Nil(t, row.CreatorEmail)
}

func TestAuthorizePatientEdit(t *testing.T) {
	repoMock, svc := setupPatientTest(t)
	ctx := context.Background()

	admin := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "admin@c.org", Name: "Admin", Role: auth.RoleAdmin}
	owner := &auth.Principal{ID: "01990000-0000-7000-8000-000000000002", Email: "owner@c.org", Name: "Owner", Role: auth.RolePhysician}
	other := &auth.Principal{ID: "01990000-0000-7000-8000-000000000003", Email: "other@c.org", Name: "Other", Role: auth.RolePhysician}

	ownerUUID := uuid.MustParse(owner.ID)
	repoMock.EXPECT().Create(mock.Anything, mock.Anything).Return(&patientmodel.Patient{
		ID:        uuid.MustParse("01990000-0000-7000-8000-000000000010"),
		ClinicID:  testClinicID,
		CreatedBy: &ownerUUID,
		Status:    patientmodel.PatientStatusActive,
	}, nil).Once()

	pt, err := svc.Create(ctx, testClinicID.String(), owner.ID, validInput())
	require.NoError(t, err)

	// Admin can edit
	err = svc.AuthorizePatientEdit(ctx, admin, pt.ID.String(), uuidStrPtrTest(pt.CreatedBy), pt.Status)
	assert.NoError(t, err)

	// Owner physician can edit
	err = svc.AuthorizePatientEdit(ctx, owner, pt.ID.String(), uuidStrPtrTest(pt.CreatedBy), pt.Status)
	assert.NoError(t, err)

	// Other physician cannot edit
	err = svc.AuthorizePatientEdit(ctx, other, pt.ID.String(), uuidStrPtrTest(pt.CreatedBy), pt.Status)
	require.ErrorIs(t, err, usecase.ErrForbidden)
}
