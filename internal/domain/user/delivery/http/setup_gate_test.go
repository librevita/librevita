package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSetupGateRedirectsWhenNotOnboarded(t *testing.T) {
	env := newUserHandlerEnv(t)
	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(false, nil).Once()

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/setup", rec.Header().Get("Location"))
}

func TestSetupGatePassesWhenOnboarded(t *testing.T) {
	env := newUserHandlerEnv(t)
	env.setupRepo.EXPECT().IsOnboarded(mock.Anything).Return(true, nil).Once()

	e := echo.New()
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "home") }, env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "home", rec.Body.String())
}

func TestSetupGateExemptsSetupPath(t *testing.T) {
	env := newUserHandlerEnv(t)

	e := echo.New()
	e.GET("/setup", func(c echo.Context) error { return c.String(http.StatusOK, "setup") }, env.handler.SetupGate())

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "setup", rec.Body.String())
}
