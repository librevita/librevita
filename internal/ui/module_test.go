package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRegisterStatic(t *testing.T) {
	e := echo.New()
	registerStatic(e)

	assert.NotEmpty(t, e.Routes())
	assert.NotNil(t, Module)

	// Make request to static route
	req := httptest.NewRequest(http.MethodGet, AppCSS, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Contains(t, rec.Header().Get("Cache-Control"), "immutable")
}
