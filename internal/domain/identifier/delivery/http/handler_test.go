package http_test

import (
	"context"
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
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	deliveryhttp "librevita.org/internal/domain/identifier/delivery/http"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/internal/domain/identifier/usecase"
	"librevita.org/pkg/log"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	policymocks "librevita.org/tests/mocks/core/policy"
	usecasemocks "librevita.org/tests/mocks/domain/identifier/usecase"
)

var testAdminID = uuid.MustParse("01990000-0000-7000-8000-00000000000a")

type httpTestEnv struct {
	echo       *echo.Echo
	sessions   *auth.SessionManager
	systemsSvc *usecasemocks.MockSystemsService
}

func newHTTPEnv(t *testing.T) *httpTestEnv {
	t.Helper()
	logger := log.Nop()

	sessionRepoMock := authmocks.NewMockSessionRepository(t)
	sessionRepoMock.EXPECT().CleanupExpired(mock.Anything, mock.Anything).Return(nil).Maybe()
	sessionRepoMock.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	sessionRepoMock.EXPECT().GetActive(mock.Anything, mock.Anything, mock.Anything).Return(&auth.SessionRecord{
		User: &auth.SessionUser{
			ID:     testAdminID,
			Email:  "admin@example.org",
			Name:   "Admin",
			Role:   auth.RoleAdmin,
			Active: true,
		},
	}, nil).Maybe()

	sessions, err := auth.NewSessionManager(sessionRepoMock, &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)

	auditRepoMock := auditmocks.NewMockRepository(t)
	auditRepoMock.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepoMock.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditLogger, err := audit.NewLogger(auditRepoMock, logger)
	require.NoError(t, err)

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

	policies, err := policy.NewPolicyEngine(policyRepoMock, logger)
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	systemsSvc := usecasemocks.NewMockSystemsService(t)
	h := deliveryhttp.NewHandler(systemsSvc, csrf, auditLogger)

	e := echo.New()
	admin := []echo.MiddlewareFunc{
		server.RequireAuth(sessions, logger),
		server.RequirePolicy(policies, auditLogger, logger, "admin.view"),
	}
	e.GET("/identifier-systems", h.IdentifierSystemsPage, admin...)
	e.POST("/identifier-systems", h.IdentifierSystemCreate, admin...)
	e.POST("/identifier-systems/:id", h.IdentifierSystemUpdate, admin...)
	e.POST("/identifier-systems/:id/active", h.IdentifierSystemSetActive, admin...)
	e.GET("/identifier-systems/check-fields", h.SystemCheckFields, admin...)

	return &httpTestEnv{
		echo:       e,
		sessions:   sessions,
		systemsSvc: systemsSvc,
	}
}

func adminSession(t *testing.T, sessions *auth.SessionManager) *http.Cookie {
	t.Helper()
	token, err := sessions.Create(context.Background(), auth.Principal{
		ID: testAdminID.String(), Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin,
	})
	require.NoError(t, err)
	return sessions.Cookie(token)
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

func getWithCookie(t *testing.T, e *echo.Echo, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestIdentifierSystemsAdmin(t *testing.T) {
	env := newHTTPEnv(t)
	cookie := adminSession(t, env.sessions)

	systemID := uuid.MustParse("01990000-0000-7000-8000-000000000055")
	cedulaSystem := &identifiermodel.IdentifierSystem{
		ID:               systemID,
		System:           "urn:librevita:id:py:cedula",
		DisplayName:      "Cédula de Identidad (Paraguay)",
		Pattern:          "[0-9]{8}",
		Transform:        identifiermodel.TransformDigits,
		CheckAlgorithm:   identifiermodel.CheckMod11Desc,
		CheckBaseLen:     7,
		CheckDVCount:     1,
		CheckStartWeight: 8,
		Active:           true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Create Paraguayan cédula
	env.systemsSvc.EXPECT().Create(mock.Anything, testAdminID.String(), mock.MatchedBy(func(in usecase.SystemInput) bool {
		return in.System == "urn:librevita:id:py:cedula"
	})).Return(cedulaSystem, nil).Once()

	rec := postForm(t, env.echo, "/identifier-systems", cookie, url.Values{
		"system":             {"urn:librevita:id:py:cedula"},
		"display_name":       {"Cédula de Identidad (Paraguay)"},
		"pattern":            {"[0-9]{8}"},
		"transform":          {"digits"},
		"check_algorithm":    {"mod11_desc"},
		"check_base_len":     {"7"},
		"check_dv_count":     {"1"},
		"check_start_weight": {"8"},
	})
	assert.Equal(t, http.StatusFound, rec.Code)

	// The catalog lists it
	env.systemsSvc.EXPECT().List(mock.Anything).Return([]*identifiermodel.IdentifierSystem{cedulaSystem}, nil).Once()
	page := getWithCookie(t, env.echo, "/identifier-systems", cookie)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Cédula de Identidad")

	// Invalid regex is rejected inline
	env.systemsSvc.EXPECT().Create(mock.Anything, testAdminID.String(), mock.MatchedBy(func(in usecase.SystemInput) bool {
		return in.Pattern == "["
	})).Return(nil, &usecase.ValidationError{Msg: "not a valid regex: error parsing regexp"}).Once()

	bad := postForm(t, env.echo, "/identifier-systems", cookie, url.Values{
		"system":          {"urn:librevita:id:x:bad"},
		"display_name":    {"Bad"},
		"pattern":         {"["},
		"transform":       {"none"},
		"check_algorithm": {"none"},
	})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "not a valid regex")

	// URN outside namespace is rejected
	env.systemsSvc.EXPECT().Create(mock.Anything, testAdminID.String(), mock.MatchedBy(func(in usecase.SystemInput) bool {
		return in.System == "com.example:doc"
	})).Return(nil, &usecase.ValidationError{Msg: "system URI must start with \"urn:librevita:id:\""}).Once()

	outside := postForm(t, env.echo, "/identifier-systems", cookie, url.Values{
		"system":          {"com.example:doc"},
		"display_name":    {"Outside"},
		"pattern":         {"[0-9]{8}"},
		"transform":       {"none"},
		"check_algorithm": {"none"},
	})
	assert.Equal(t, http.StatusOK, outside.Code)
	assert.Contains(t, outside.Body.String(), "urn:librevita:id:")

	// Toggle cédula inactive
	inactiveCedula := *cedulaSystem
	inactiveCedula.Active = false
	env.systemsSvc.EXPECT().SystemByID(mock.Anything, systemID.String()).Return(cedulaSystem, nil).Once()
	env.systemsSvc.EXPECT().SetActive(mock.Anything, systemID.String(), false).Return(nil).Once()
	env.systemsSvc.EXPECT().SystemByID(mock.Anything, systemID.String()).Return(&inactiveCedula, nil).Once()

	toggle := postForm(t, env.echo, "/identifier-systems/"+systemID.String()+"/active", cookie, url.Values{})
	assert.Equal(t, http.StatusOK, toggle.Code)
	assert.Contains(t, toggle.Body.String(), "Inactive")

	// Check fields partial renders only for chosen algorithm
	fields := getWithCookie(t, env.echo, "/identifier-systems/check-fields?check_algorithm=mod11_desc", cookie)
	assert.Equal(t, http.StatusOK, fields.Code)
	assert.Contains(t, fields.Body.String(), "Start weight")

	none := getWithCookie(t, env.echo, "/identifier-systems/check-fields?check_algorithm=none", cookie)
	assert.Equal(t, http.StatusOK, none.Code)
	assert.NotContains(t, none.Body.String(), "Start weight")
}
