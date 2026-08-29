package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

func TestClientIPIgnoresXFFWithoutTrustedProxies(t *testing.T) {
	e := New(auth.NewCSRF(&config.Config{Mode: "development"}), &config.Config{Mode: "development"}, testLogger(), middlewareSkippers{})
	e.GET("/ip", func(c echo.Context) error { return c.String(http.StatusOK, c.RealIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "203.0.113.10", rec.Body.String())
}

func TestClientIPTrustsXFFFromTrustedProxy(t *testing.T) {
	cfg := &config.Config{Mode: "development", TrustedProxies: "10.0.0.0/8"}
	e := New(auth.NewCSRF(cfg), cfg, testLogger(), middlewareSkippers{})
	e.GET("/ip", func(c echo.Context) error { return c.String(http.StatusOK, c.RealIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "198.51.100.1", rec.Body.String())
}

func TestClientIPExtractorIPv6Literal(t *testing.T) {
	extract := clientIPExtractor("2001:db8::1")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[2001:db8::1]:443"
	req.Header.Set("X-Forwarded-For", "2001:db8::cafe")
	assert.Equal(t, "2001:db8::cafe", extract(req))
}

func TestHostIPNetMasks(t *testing.T) {
	v4 := hostIPNet(net.ParseIP("192.0.2.1"))
	require.NotNil(t, v4)
	ones, bits := v4.Mask.Size()
	assert.Equal(t, 32, ones)
	assert.Equal(t, 32, bits)

	v6 := hostIPNet(net.ParseIP("2001:db8::1"))
	require.NotNil(t, v6)
	ones, bits = v6.Mask.Size()
	assert.Equal(t, 128, ones)
	assert.Equal(t, 128, bits)
}
