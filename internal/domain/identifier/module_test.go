package identifier

import (
	"context"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/domain/identifier/delivery/http"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	auditmocks "librevita.org/internal/test/mock/core/audit"
	authmocks "librevita.org/internal/test/mock/core/auth"
	policymocks "librevita.org/internal/test/mock/core/policy"
	identifiermocks "librevita.org/internal/test/mock/domain/identifier/model"
	"librevita.org/pkg/log"
)

func TestIdentifierModuleLifecycleAndRoutes(t *testing.T) {
	logger := log.Nop()
	lc := fxtest.NewLifecycle(t)
	reg := identifiermodel.NewRegistry()
	repo := identifiermocks.NewMockSystemRepository(t)

	repo.EXPECT().SeedDefaults(mock.Anything).Return(nil).Once()
	repo.EXPECT().ListActive(mock.Anything).Return(nil, nil).Once()

	loadIdentifierSystems(lc, reg, repo, logger)
	require.NoError(t, lc.Start(context.Background()))
	require.NoError(t, lc.Stop(context.Background()))

	// Routes
	e := echo.New()
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
