package http_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	"librevita.org/internal/domain/user/usecase"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	policymocks "librevita.org/tests/mocks/core/policy"
	storagemocks "librevita.org/tests/mocks/core/storage"
	clinicmocks "librevita.org/tests/mocks/domain/clinic/model"
	patientmocks "librevita.org/tests/mocks/domain/patient/model"
	usermocks "librevita.org/tests/mocks/domain/user/model"
)

type userHandlerTestEnv struct {
	handler     *httphandler.Handler
	sessions    *auth.SessionManager
	sessionRepo *authmocks.MockSessionRepository
	setupRepo   *usermocks.MockSetupRepository
	svc         *usecase.Service
}

func newUserHandlerEnv(t *testing.T) *userHandlerTestEnv {
	t.Helper()
	log := slog.New(slog.DiscardHandler)

	sessionRepo := authmocks.NewMockSessionRepository(t)
	sessionRepo.EXPECT().CleanupExpired(mock.Anything, mock.Anything).Return(nil).Maybe()
	sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	sessions, err := auth.NewSessionManager(sessionRepo, &config.Config{Mode: "development"}, log)
	require.NoError(t, err)

	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditLogger, err := audit.NewLogger(auditRepo, log)
	require.NoError(t, err)

	userRepo := usermocks.NewMockUserRepository(t)
	roleRepo := usermocks.NewMockRoleRepository(t)
	specialtyRepo := usermocks.NewMockSpecialtyRepository(t)
	staffReqRepo := usermocks.NewMockStaffRequestRepository(t)
	setupRepo := usermocks.NewMockSetupRepository(t)

	svc := usecase.NewService(userRepo, roleRepo, specialtyRepo, staffReqRepo, setupRepo, sessions, auditLogger, log)
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})

	policyRepo := policymocks.NewMockRepository(t)
	policyRepo.EXPECT().SeedDefaults(mock.Anything, mock.Anything).Return(nil).Maybe()
	var defaultRows []policy.PolicyRow
	for name, expr := range policy.DefaultPolicies {
		defaultRows = append(defaultRows, policy.PolicyRow{
			Name:       name,
			Expression: expr,
		})
	}
	policyRepo.EXPECT().List(mock.Anything).Return(defaultRows, nil).Maybe()
	policies, err := policy.NewPolicyEngine(policyRepo, log)
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	patientRepo := patientmocks.NewMockPatientRepository(t)
	patientSvc := usecase.NewService(userRepo, roleRepo, specialtyRepo, staffReqRepo, setupRepo, sessions, auditLogger, log)
	_ = patientRepo
	_ = patientSvc

	clinicRepo := clinicmocks.NewMockRepository(t)
	clocks := clinicusecase.NewClockProvider(clinicRepo)

	fileStore := storagemocks.NewMockStore(t)
	fileIndex := storagemocks.NewMockIndexRepository(t)
	_ = fileStore
	_ = fileIndex

	h := httphandler.NewHandler(svc, nil, nil, nil, csrf, sessions, policies, auditLogger, clocks, nil, &config.Config{Mode: "development"}, log)
	return &userHandlerTestEnv{
		handler:     h,
		sessions:    sessions,
		sessionRepo: sessionRepo,
		setupRepo:   setupRepo,
		svc:         svc,
	}
}

func TestLogoutSurfacesRevocationFailure(t *testing.T) {
	env := newUserHandlerEnv(t)

	token, err := env.sessions.Create(context.Background(), auth.Principal{
		ID: "01990000-0000-7000-8000-000000000001", Email: "ana@example.org", Name: "Ana", Role: auth.RoleAdmin,
	})
	require.NoError(t, err)

	// Simulate database failure during session deletion
	env.sessionRepo.EXPECT().Delete(mock.Anything, mock.Anything).Return(errors.New("db outage")).Once()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(env.sessions.Cookie(token))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = env.handler.Logout(c)
	assert.Error(t, err, "Logout must return an error when revocation fails")
}
