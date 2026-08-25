package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/clinicctx"
)

func attachClinic(onboarded bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var at *time.Time
			if onboarded {
				now := time.Now()
				at = &now
			}
			ctx := clinicctx.WithClinic(c.Request().Context(), &clinicctx.Clinic{
				ID:          clinicctx.TestClinicID,
				Slug:        "test",
				Name:        "Test Clinic",
				Timezone:    "America/Sao_Paulo",
				OnboardedAt: at,
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func TestSetupGateRedirectsWhenNotOnboarded(t *testing.T) {
	env := newUserHandlerEnv(t)

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, attachClinic(false), env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/setup", rec.Header().Get("Location"))
}

func TestSetupGatePassesWhenOnboarded(t *testing.T) {
	env := newUserHandlerEnv(t)

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, attachClinic(true), env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "home", rec.Body.String())
}

func TestSetupGateExemptsSetupPath(t *testing.T) {
	env := newUserHandlerEnv(t)

	e := echo.New()
	e.GET("/setup", func(c echo.Context) error { return c.String(http.StatusOK, "setup") }, attachClinic(false), env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "setup", rec.Body.String())
}
