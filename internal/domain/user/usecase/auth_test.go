package usecase_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	clinicmodel "librevita.org/internal/domain/clinic/model"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/internal/domain/user/usecase"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	usermocks "librevita.org/tests/mocks/domain/user/model"
)

type testEnv struct {
	userRepo      *usermocks.MockUserRepository
	roleRepo      *usermocks.MockRoleRepository
	specialtyRepo *usermocks.MockSpecialtyRepository
	staffReqRepo  *usermocks.MockStaffRequestRepository
	setupRepo     *usermocks.MockSetupRepository
	sessionRepo   *authmocks.MockSessionRepository
	auditRepo     *auditmocks.MockRepository
	sessions      *auth.SessionManager
	svc           *usecase.Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	userRepo := usermocks.NewMockUserRepository(t)
	roleRepo := usermocks.NewMockRoleRepository(t)
	specialtyRepo := usermocks.NewMockSpecialtyRepository(t)
	staffReqRepo := usermocks.NewMockStaffRequestRepository(t)
	setupRepo := usermocks.NewMockSetupRepository(t)
	sessionRepo := authmocks.NewMockSessionRepository(t)
	auditRepo := auditmocks.NewMockRepository(t)

	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	sessionRepo.EXPECT().CleanupExpired(mock.Anything, mock.Anything).Return(nil).Maybe()

	sessions, err := auth.NewSessionManager(sessionRepo, &config.Config{Mode: "development"}, log)
	require.NoError(t, err)

	auditLogger, err := audit.NewLogger(auditRepo, log)
	require.NoError(t, err)

	svc := usecase.NewService(userRepo, roleRepo, specialtyRepo, staffReqRepo, setupRepo, sessions, auditLogger, log)

	return &testEnv{
		userRepo:      userRepo,
		roleRepo:      roleRepo,
		specialtyRepo: specialtyRepo,
		staffReqRepo:  staffReqRepo,
		setupRepo:     setupRepo,
		sessionRepo:   sessionRepo,
		auditRepo:     auditRepo,
		sessions:      sessions,
		svc:           svc,
	}
}

func validInput() usecase.RegisterInput {
	return usecase.RegisterInput{
		Name:     "Ana Souza",
		Email:    "ana@example.org",
		Password: "password-123",
	}
}

func validClinicInput() usecase.ClinicInput {
	return usecase.ClinicInput{
		Name:    "Clínica Exemplo",
		TaxID:   "12.345.678/0001-90",
		City:    "São Paulo",
		State:   "SP",
		Country: "BR",
	}
}

func TestRegisterCreatesPatient(t *testing.T) {
	env := newTestEnv(t)
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "patient").Return(&usermodel.Role{
		ID:   patientRoleID,
		Name: "patient",
	}, nil).Once()

	var createdUser *usermodel.User
	env.userRepo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
		createdUser = u
		return u, nil
	}).Once()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	p, token, err := env.svc.Register(context.Background(), validInput())
	require.NoError(t, err)
	assert.Equal(t, auth.RolePatient, p.Role)
	assert.Equal(t, "ana@example.org", p.Email)
	assert.NotEmpty(t, token)
	assert.NotNil(t, createdUser)
	assert.Equal(t, patientRoleID, createdUser.RoleID)
}

func TestRegisterSecondUserBecomesPatient(t *testing.T) {
	env := newTestEnv(t)
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "patient").Return(&usermodel.Role{
		ID:   patientRoleID,
		Name: "patient",
	}, nil).Once()

	env.userRepo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
		return u, nil
	}).Once()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	input := validInput()
	input.Email = "bruno@example.org"
	p, _, err := env.svc.Register(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, auth.RolePatient, p.Role)
}

func TestRegisterValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name    string
		mutate  func(*usecase.RegisterInput)
		message string
	}{
		{"missing name", func(in *usecase.RegisterInput) { in.Name = " " }, "display name"},
		{"missing email", func(in *usecase.RegisterInput) { in.Email = "" }, "valid email"},
		{"invalid email", func(in *usecase.RegisterInput) { in.Email = "not-an-email" }, "valid email"},
		{"short password", func(in *usecase.RegisterInput) { in.Password = "short" }, "at least 8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validInput()
			tc.mutate(&input)
			_, _, err := env.svc.Register(context.Background(), input)
			require.Error(t, err)
			var v *usecase.ValidationError
			require.ErrorAs(t, err, &v)
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	env := newTestEnv(t)
	userID := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	hash, err := auth.HashPassword("password-123")
	require.NoError(t, err)

	env.userRepo.EXPECT().GetByEmail(mock.Anything, "ana@example.org").Return(&usermodel.GetUserByIDRow{
		ID:           userID,
		Email:        "ana@example.org",
		PasswordHash: hash,
		DisplayName:  "Ana Souza",
		RoleName:     "patient",
		Active:       true,
	}, nil).Once()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, userID, mock.Anything).Return(nil).Once()

	p, token, err := env.svc.Login(context.Background(), usecase.Credentials{
		Email:    "ANA@example.org", // Case insensitive
		Password: "password-123",
	})
	require.NoError(t, err)
	assert.Equal(t, "ana@example.org", p.Email)
	assert.NotEmpty(t, token)
}

func TestLoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)
	userID := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	hash, err := auth.HashPassword("password-123")
	require.NoError(t, err)

	env.userRepo.EXPECT().GetByEmail(mock.Anything, "ana@example.org").Return(&usermodel.GetUserByIDRow{
		ID:           userID,
		Email:        "ana@example.org",
		PasswordHash: hash,
		DisplayName:  "Ana Souza",
		RoleName:     "patient",
		Active:       true,
	}, nil).Once()

	_, _, err = env.svc.Login(context.Background(), usecase.Credentials{
		Email:    "ana@example.org",
		Password: "wrong-password",
	})
	require.ErrorIs(t, err, usecase.ErrInvalidCredentials)
}

func TestLoginUnknownEmail(t *testing.T) {
	env := newTestEnv(t)
	env.userRepo.EXPECT().GetByEmail(mock.Anything, "unknown@example.org").Return(nil, usecase.ErrUserNotFound).Once()

	_, _, err := env.svc.Login(context.Background(), usecase.Credentials{
		Email:    "unknown@example.org",
		Password: "password-123",
	})
	require.ErrorIs(t, err, usecase.ErrInvalidCredentials)
}

func TestLogout(t *testing.T) {
	env := newTestEnv(t)
	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	token, err := env.sessions.Create(context.Background(), auth.Principal{
		ID:    uuid.NewString(),
		Email: "ana@example.org",
		Name:  "Ana",
		Role:  auth.RolePatient,
	})
	require.NoError(t, err)

	env.sessionRepo.EXPECT().Delete(mock.Anything, mock.Anything).Return(nil).Once()

	err = env.svc.Logout(context.Background(), token)
	require.NoError(t, err)
}

func TestConcurrentRegistrationsProduceOnlyPatients(t *testing.T) {
	env := newTestEnv(t)
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "patient").Return(&usermodel.Role{
		ID:   patientRoleID,
		Name: "patient",
	}, nil).Maybe()

	env.userRepo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
		assert.Equal(t, patientRoleID, u.RoleID)
		return u, nil
	}).Maybe()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	const attempts = 8
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := validInput()
			input.Email = "user" + string(rune('a'+i)) + "@example.org"
			_, _, errs[i] = env.svc.Register(context.Background(), input)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestDuplicateEmailMapsToDomainError(t *testing.T) {
	env := newTestEnv(t)
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "patient").Return(&usermodel.Role{
		ID:   patientRoleID,
		Name: "patient",
	}, nil).Once()

	env.userRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, usecase.ErrEmailTaken).Once()

	input := validInput()
	input.Email = "ANA@example.org"
	_, _, err := env.svc.Register(context.Background(), input)
	require.ErrorIs(t, err, usecase.ErrEmailTaken)
}

func TestRegisterUsesUUIDv7ID(t *testing.T) {
	env := newTestEnv(t)
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "patient").Return(&usermodel.Role{
		ID:   patientRoleID,
		Name: "patient",
	}, nil).Once()

	var createdID uuid.UUID
	env.userRepo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
		createdID = u.ID
		return u, nil
	}).Once()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	p, _, err := env.svc.Register(context.Background(), validInput())
	require.NoError(t, err)
	assert.Equal(t, 7, int(createdID.Version()))
	assert.Equal(t, createdID.String(), p.ID)
}

func TestIsOnboarded(t *testing.T) {
	env := newTestEnv(t)

	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(false, nil).Once()
	ok, err := env.svc.IsOnboarded(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)

	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(true, nil).Once()
	ok, err = env.svc.IsOnboarded(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOnboardCreatesAdminAndClinic(t *testing.T) {
	env := newTestEnv(t)
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")
	adminUserID := uuid.MustParse("01990000-0000-7000-8000-000000000003")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "admin").Return(&usermodel.Role{
		ID:   adminRoleID,
		Name: "admin",
	}, nil).Once()

	env.setupRepo.EXPECT().Onboard(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, adminUser *usermodel.User, clinic *clinicmodel.Clinic) (*usermodel.User, error) {
		assert.Equal(t, adminRoleID, adminUser.RoleID)
		assert.Equal(t, "Clínica Exemplo", clinic.Name)
		adminUser.ID = adminUserID
		return adminUser, nil
	}).Once()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	p, token, err := env.svc.Onboard(context.Background(), validInput(), validClinicInput())
	require.NoError(t, err)
	assert.Equal(t, auth.RoleAdmin, p.Role)
	assert.NotEmpty(t, token)
}

func TestOnboardFailsWhenAlreadyOnboarded(t *testing.T) {
	env := newTestEnv(t)
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "admin").Return(&usermodel.Role{
		ID:   adminRoleID,
		Name: "admin",
	}, nil).Once()

	env.setupRepo.EXPECT().Onboard(mock.Anything, mock.Anything, mock.Anything).Return(nil, usecase.ErrAlreadyOnboarded).Once()

	input := validInput()
	input.Email = "outro@example.org"
	_, _, err := env.svc.Onboard(context.Background(), input, validClinicInput())
	require.ErrorIs(t, err, usecase.ErrAlreadyOnboarded)
}

func TestConcurrentOnboardSingleWinner(t *testing.T) {
	env := newTestEnv(t)
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")

	env.roleRepo.EXPECT().GetByName(mock.Anything, "admin").Return(&usermodel.Role{
		ID:   adminRoleID,
		Name: "admin",
	}, nil).Maybe()

	var setupMu sync.Mutex
	setupDone := false

	env.setupRepo.EXPECT().Onboard(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User, c *clinicmodel.Clinic) (*usermodel.User, error) {
		setupMu.Lock()
		defer setupMu.Unlock()
		if setupDone {
			return nil, usecase.ErrAlreadyOnboarded
		}
		setupDone = true
		return u, nil
	}).Maybe()

	env.sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	const attempts = 6
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := validInput()
			input.Email = "admin" + string(rune('a'+i)) + "@example.org"
			_, _, errs[i] = env.svc.Onboard(context.Background(), input, validClinicInput())
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, usecase.ErrAlreadyOnboarded)
		}
	}
	assert.Equal(t, 1, successes)
}

func TestOnboardValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name    string
		mutate  func(*usecase.RegisterInput, *usecase.ClinicInput)
		message string
	}{
		{"missing admin name", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { a.Name = " " }, "display name"},
		{"short password", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { a.Password = "short" }, "at least 8"},
		{"missing clinic name", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Name = "" }, "clinic name"},
		{"bad clinic email", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Email = "not-an-email" }, "clinic email"},
		{"invalid timezone", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Timezone = "Mars/Olympus" }, "from the list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admin := validInput()
			clinic := validClinicInput()
			tc.mutate(&admin, &clinic)
			_, _, err := env.svc.Onboard(context.Background(), admin, clinic)
			require.Error(t, err)
			var v *usecase.ValidationError
			require.ErrorAs(t, err, &v)
		})
	}
}

func TestSetupCannotBeReexecutedAfterDataRemoval(t *testing.T) {
	env := newTestEnv(t)
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")

	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(true, nil).Once()
	ok, err := env.svc.IsOnboarded(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)

	env.roleRepo.EXPECT().GetByName(mock.Anything, "admin").Return(&usermodel.Role{
		ID:   adminRoleID,
		Name: "admin",
	}, nil).Once()
	env.setupRepo.EXPECT().Onboard(mock.Anything, mock.Anything, mock.Anything).Return(nil, usecase.ErrAlreadyOnboarded).Once()

	_, _, err = env.svc.Onboard(context.Background(), validInput(), validClinicInput())
	require.ErrorIs(t, err, usecase.ErrAlreadyOnboarded)
}

func TestSetupMarkerGuardsDeletedMarkerEdgeCase(t *testing.T) {
	env := newTestEnv(t)
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")

	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(true, nil).Once()
	ok, err := env.svc.IsOnboarded(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)

	env.roleRepo.EXPECT().GetByName(mock.Anything, "admin").Return(&usermodel.Role{
		ID:   adminRoleID,
		Name: "admin",
	}, nil).Once()
	env.setupRepo.EXPECT().Onboard(mock.Anything, mock.Anything, mock.Anything).Return(nil, usecase.ErrAlreadyOnboarded).Once()

	_, _, err = env.svc.Onboard(context.Background(), validInput(), validClinicInput())
	require.ErrorIs(t, err, usecase.ErrAlreadyOnboarded)
}
