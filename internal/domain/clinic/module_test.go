package clinic

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/config"
	clinicmocks "librevita.org/internal/test/mock/domain/clinic/model"
	"librevita.org/pkg/log"
)

func TestRegisterHostMiddleware(t *testing.T) {
	e := echo.New()
	cfg := &config.Config{BaseDomain: "lv.test"}
	repo := clinicmocks.NewMockRepository(t)
	logger := log.Nop()

	registerHostMiddleware(e, cfg, repo, nil, logger)
	assert.NotNil(t, Module)
}
