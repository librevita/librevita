package acme

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestHTTP01RegistryAndHandler(t *testing.T) {
	registry := NewHTTP01Registry()

	// Initial lookup empty
	_, ok := registry.Lookup("tok123")
	assert.False(t, ok)

	// Register token
	registry.Register("tok123", "auth.key.123")
	val, ok := registry.Lookup("tok123")
	assert.True(t, ok)
	assert.Equal(t, "auth.key.123", val)

	// Handler with Echo
	e := echo.New()
	e.GET("/.well-known/acme-challenge/:token", registry.Handler())

	// Test found
	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok123", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "auth.key.123", rec.Body.String())
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))

	// Test unregister
	registry.Unregister("tok123")
	_, ok = registry.Lookup("tok123")
	assert.False(t, ok)

	// Test not found
	req = httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok123", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
