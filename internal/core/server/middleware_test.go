package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/ui"
	"librevita.org/pkg/log"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	policymocks "librevita.org/tests/mocks/core/policy"
)

func testLogger() log.Logger { return log.Nop() }

func newMockSessionManager(t *testing.T) (*auth.SessionManager, *authmocks.MockSessionRepository) {
	t.Helper()
	sessionRepo := authmocks.NewMockSessionRepository(t)
	sessionRepo.EXPECT().CleanupExpired(mock.Anything, mock.Anything).Return(nil).Maybe()
	sessionRepo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	mgr, err := auth.NewSessionManager(sessionRepo, &config.Config{Mode: "development"}, testLogger())
	require.NoError(t, err)
	return mgr, sessionRepo
}

func newMockAuditLogger(t *testing.T) (*audit.Logger, *auditmocks.MockRepository) {
	t.Helper()
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Maybe()
	auditRepo.EXPECT().Record(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	l, err := audit.NewLogger(auditRepo, testLogger())
	require.NoError(t, err)
	return l, auditRepo
}

func newMockPolicyEngine(t *testing.T) (*policy.PolicyEngine, *policymocks.MockRepository) {
	t.Helper()
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

	pe, err := policy.NewPolicyEngine(policyRepo, testLogger())
	require.NoError(t, err)
	require.NoError(t, pe.Load(context.Background()))
	return pe, policyRepo
}

func TestRequireAuthRedirectsAnonymous(t *testing.T) {
	sessions, _ := newMockSessionManager(t)

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "ok") }, RequireAuth(sessions, testLogger()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, LoginPath, rec.Header().Get("Location"))
}

func TestRequireAuthAcceptsValidSession(t *testing.T) {
	sessions, sessionRepo := newMockSessionManager(t)
	userID := ident.MustParseUser("01990000-0000-7000-8000-000000000001")

	token, err := sessions.Create(context.Background(), auth.Principal{
		ID:    userID.String(),
		Email: "user@example.org",
		Name:  "Test User",
		Role:  auth.RoleAdmin,
	})
	require.NoError(t, err)

	sessionRepo.EXPECT().GetActive(mock.Anything, mock.Anything, mock.Anything).Return(&auth.SessionRecord{
		User: &auth.SessionUser{
			ID:     userID.UUID(),
			Email:  "user@example.org",
			Name:   "Test User",
			Role:   auth.RoleAdmin,
			Active: true,
		},
	}, nil).Once()

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		p := Principal(c)
		require.NotNil(t, p, "principal missing from context")
		assert.Equal(t, userID.String(), p.ID)
		return c.String(http.StatusOK, "ok")
	}, RequireAuth(sessions, testLogger()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(sessions.Cookie(token))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePolicyAllowsAndDenies(t *testing.T) {
	pe, _ := newMockPolicyEngine(t)
	auditLogger, _ := newMockAuditLogger(t)

	for _, tc := range []struct {
		name string
		role auth.Role
		code int
	}{
		{"admin allowed", auth.RoleAdmin, http.StatusOK},
		{"patient denied", auth.RolePatient, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
				func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c echo.Context) error {
						c.Set(principalKey, &auth.Principal{
							ID:    "01990000-0000-7000-8000-000000000001",
							Email: "u@example.org",
							Name:  "User",
							Role:  tc.role,
						})
						return next(c)
					}
				},
				RequirePolicy(pe, auditLogger, testLogger(), "admin.view"))

			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tc.code, rec.Code)
		})
	}
}

func TestRequirePolicyRedirectsWithoutPrincipal(t *testing.T) {
	pe, _ := newMockPolicyEngine(t)
	auditLogger, _ := newMockAuditLogger(t)

	e := echo.New()
	e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
		RequirePolicy(pe, auditLogger, testLogger(), "admin.view"))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestRequirePolicyDenialIsAudited(t *testing.T) {
	pe, _ := newMockPolicyEngine(t)
	auditRepo := auditmocks.NewMockRepository(t)
	auditRepo.EXPECT().LastSignature(mock.Anything).Return("", nil).Once()
	auditRepo.EXPECT().Record(mock.Anything, mock.MatchedBy(func(ev audit.Event) bool {
		return ev.Action == "authorize" && ev.Resource == "policy:admin.view" && ev.Result == audit.AuditResultFailure
	}), mock.Anything, mock.Anything).Return(nil).Once()

	auditLogger, err := audit.NewLogger(auditRepo, testLogger())
	require.NoError(t, err)

	e := echo.New()
	e.GET("/admin", func(c echo.Context) error { return c.String(http.StatusOK, "ok") },
		func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Set(principalKey, &auth.Principal{
					ID:    "01990000-0000-7000-8000-000000000001",
					Email: "u@example.org",
					Name:  "User",
					Role:  auth.RolePatient,
				})
				return next(c)
			}
		},
		RequirePolicy(pe, auditLogger, testLogger(), "admin.view"))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestNotFoundRedirectsAnonymous(t *testing.T) {
	sessions, _ := newMockSessionManager(t)
	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/no-such-page", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, LoginPath+"?next=%2Fno-such-page", rec.Header().Get("Location"))
}

func TestNotFoundAuthenticated(t *testing.T) {
	sessions, sessionRepo := newMockSessionManager(t)
	userID := ident.MustParseUser("01990000-0000-7000-8000-000000000001")

	token, err := sessions.Create(context.Background(), auth.Principal{
		ID:    userID.String(),
		Email: "user@example.org",
		Name:  "Test User",
		Role:  auth.RoleAdmin,
	})
	require.NoError(t, err)

	sessionRepo.EXPECT().GetActive(mock.Anything, mock.Anything, mock.Anything).Return(&auth.SessionRecord{
		User: &auth.SessionUser{
			ID:     userID.UUID(),
			Email:  "user@example.org",
			Name:   "Test User",
			Role:   auth.RoleAdmin,
			Active: true,
		},
	}, nil).Once()

	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/no-such-page", nil)
	req.AddCookie(sessions.Cookie(token))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNotFoundPublicPaths(t *testing.T) {
	sessions, _ := newMockSessionManager(t)
	e := echo.New()
	registerNotFound(e, sessions)

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSecurityHeadersAreStrict(t *testing.T) {
	e := New(auth.NewCSRF(&config.Config{Mode: "development"}), &config.Config{Mode: "development"}, testLogger(), middlewareSkippers{})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	wantCSP := "default-src 'self'; script-src 'self' '" + ui.ThemeScriptHash + "'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	assert.Equal(t, wantCSP, rec.Header().Get("Content-Security-Policy"))

	for _, h := range []string{
		"X-Content-Type-Options",
		"Referrer-Policy",
		"X-Frame-Options",
		"Cross-Origin-Resource-Policy",
		"Permissions-Policy",
	} {
		assert.NotEmpty(t, rec.Header().Get(h), "missing security header %s", h)
	}
	assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}

func TestSecurityHeadersHSTSIsConfigurable(t *testing.T) {
	e := New(auth.NewCSRF(&config.Config{Mode: "development"}), &config.Config{Mode: "development", HSTSMaxAge: 31536000}, testLogger(), middlewareSkippers{})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "max-age=31536000", rec.Header().Get("Strict-Transport-Security"))
}

func TestBodyLimitRaisesAvatarUploadCap(t *testing.T) {
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})
	e := New(csrf, &config.Config{Mode: "development"}, testLogger(), middlewareSkippers{
		BodyLimit: []middleware.Skipper{
			func(c echo.Context) bool { return c.Path() == "/profile/avatar" },
		},
	})
	e.POST("/profile/avatar", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.POST("/profile", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	big := strings.Repeat("x", 1<<20+1024)

	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", strings.NewReader(big))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code, "avatar upload was capped by global 1M body limit")

	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(big))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
