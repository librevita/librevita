package user

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	auditmocks "librevita.org/internal/test/mock/core/audit"
	authmocks "librevita.org/internal/test/mock/core/auth"
	policymocks "librevita.org/internal/test/mock/core/policy"
	"librevita.org/pkg/log"
)

func TestBodyLimitSkipperMatchesAvatarRoute(t *testing.T) {
	e := echo.New()
	e.POST("/profile/avatar", func(c echo.Context) error {
		if !bodyLimitSkipper()(c) {
			t.Fatal("expected body-limit skipper to match avatar upload route")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRegisterRoutes(t *testing.T) {
	e := echo.New()
	logger := log.Nop()
	repoMock := policymocks.NewMockRepository(t)
	policies, err := policy.NewPolicyEngine(repoMock, logger)
	require.NoError(t, err)

	auditRepoMock := auditmocks.NewMockRepository(t)
	auditLogger, err := audit.NewLogger(auditRepoMock, logger)
	require.NoError(t, err)

	sessRepo := authmocks.NewMockSessionRepository(t)
	sessions, err := auth.NewSessionManager(sessRepo, &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)

	h := &httphandler.Handler{}

	registerRoutes(e, h, sessions, policies, auditLogger, logger)
	assert.NotEmpty(t, e.Routes())
}
