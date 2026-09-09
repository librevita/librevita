package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/acme"
	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
)

func TestBuildHTTPRedirectServer_ACMEAndRedirect(t *testing.T) {
	cfg := &config.Config{
		TLS: config.TLSConfig{
			Enabled:      true,
			HTTPSPort:    8443,
			RedirectHTTP: true,
		},
		ACME: config.ACMEConfig{
			Enabled: true,
		},
	}

	acmeMgr, err := acme.NewManager(cfg, nil, acme.NewMockDNSProvider(), log.Nop())
	require.NoError(t, err)

	// Register a challenge token
	acmeMgr.HTTP01Registry().Register("test-tok-123", "keyauth-val-456")

	p := serverParams{
		Echo:        echo.New(),
		Config:      cfg,
		Logger:      log.Nop(),
		ACMEManager: acmeMgr,
	}

	httpServer := buildHTTPRedirectServer(p, "127.0.0.1:8080")
	handler := httpServer.Handler

	// 1. Challenge path should be answered with keyauth
	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/test-tok-123", nil)
	req.Host = "example.org"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "keyauth-val-456", rec.Body.String())
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))

	// 2. Unknown challenge token should 404
	reqUnknown := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/unknown-tok", nil)
	reqUnknown.Host = "example.org"
	recUnknown := httptest.NewRecorder()
	handler.ServeHTTP(recUnknown, reqUnknown)

	assert.Equal(t, http.StatusNotFound, recUnknown.Code)

	// 3. Normal path should redirect to HTTPS with 301
	reqNormal := httptest.NewRequest(http.MethodGet, "/login?ref=1", nil)
	reqNormal.Host = "example.org:8080"
	recNormal := httptest.NewRecorder()
	handler.ServeHTTP(recNormal, reqNormal)

	assert.Equal(t, http.StatusMovedPermanently, recNormal.Code)
	assert.Equal(t, "https://example.org:8443/login?ref=1", recNormal.Header().Get("Location"))

	// 4. Default HTTPS port (443) should not append port
	cfg.TLS.HTTPSPort = 443
	rec443 := httptest.NewRecorder()
	handler.ServeHTTP(rec443, reqNormal)
	assert.Equal(t, "https://example.org/login?ref=1", rec443.Header().Get("Location"))
}

func TestBuildTLSConfig(t *testing.T) {
	cfg := &config.Config{
		TLS: config.TLSConfig{
			Enabled: true,
		},
	}

	// Neither static cert nor ACME manager
	pNoACME := serverParams{
		Config: cfg,
	}
	_, err := buildTLSConfig(pNoACME)
	assert.Error(t, err)

	// With ACME manager
	acmeMgr, err := acme.NewManager(cfg, nil, acme.NewMockDNSProvider(), log.Nop())
	require.NoError(t, err)

	pWithACME := serverParams{
		Config:      cfg,
		ACMEManager: acmeMgr,
	}
	tlsConfig, err := buildTLSConfig(pWithACME)
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.NotNil(t, tlsConfig.GetCertificate)
}
