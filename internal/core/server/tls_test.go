package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

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

func TestBuildTLSConfig_StaticCerts(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	cfg := &config.Config{
		TLS: config.TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	}
	p := serverParams{
		Config: cfg,
	}
	tlsConfig, err := buildTLSConfig(p)
	require.NoError(t, err)
	assert.Len(t, tlsConfig.Certificates, 1)

	// Invalid cert files
	pBad := serverParams{
		Config: &config.Config{
			TLS: config.TLSConfig{
				Enabled:  true,
				CertFile: "/nonexistent.crt",
				KeyFile:  "/nonexistent.key",
			},
		},
	}
	_, err = buildTLSConfig(pBad)
	assert.Error(t, err)
}

func TestRegisterLifecycle_Hooks(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	cfg := &config.Config{
		HTTPBind: "127.0.0.1",
		HTTPPort: 0,
		TLS: config.TLSConfig{
			Enabled:   true,
			HTTPSBind: "127.0.0.1",
			HTTPSPort: 0,
		},
	}
	acmeMgr, err := acme.NewManager(cfg, nil, acme.NewMockDNSProvider(), log.Nop())
	require.NoError(t, err)

	p := serverParams{
		Lifecycle:   lc,
		Echo:        echo.New(),
		Config:      cfg,
		Logger:      log.Nop(),
		ACMEManager: acmeMgr,
	}

	registerLifecycle(p)

	// Plain HTTP lifecycle
	lcPlain := fxtest.NewLifecycle(t)
	cfgPlain := &config.Config{
		HTTPBind: "127.0.0.1",
		HTTPPort: 0,
		TLS: config.TLSConfig{
			Enabled: false,
		},
	}
	pPlain := serverParams{
		Lifecycle: lcPlain,
		Echo:      echo.New(),
		Config:    cfgPlain,
		Logger:    log.Nop(),
	}
	registerLifecycle(pPlain)

	ctx := context.Background()
	require.NoError(t, lcPlain.Start(ctx))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, lcPlain.Stop(ctx))

	// TLS lifecycle with error in buildTLSConfig
	lcErr := fxtest.NewLifecycle(t)
	cfgErr := &config.Config{
		TLS: config.TLSConfig{Enabled: true},
	}
	pErr := serverParams{
		Lifecycle: lcErr,
		Echo:      echo.New(),
		Config:    cfgErr,
		Logger:    log.Nop(),
	}
	registerLifecycle(pErr)

	// TLS lifecycle start and stop
	require.NoError(t, lc.Start(ctx))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, lc.Stop(ctx))
}
