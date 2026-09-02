package episode

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/domain/episode/delivery/http"
	"librevita.org/pkg/log"
	auditmocks "librevita.org/tests/mocks/core/audit"
	authmocks "librevita.org/tests/mocks/core/auth"
	policymocks "librevita.org/tests/mocks/core/policy"
)

func TestRegisterEpisodeRoutes(t *testing.T) {
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

	gate := func() echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}
	}

	h := &http.Handler{}
	registerHTTPRoutes(e, h, sessions, policies, auditLogger, gate, logger)

	assert.NotEmpty(t, e.Routes())
	assert.NotNil(t, Module)
}
