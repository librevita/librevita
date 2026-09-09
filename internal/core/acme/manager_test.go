package acme

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
)

func TestManager_GetCertificateAndRenewal(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:         true,
			Email:           "admin@example.org",
			Challenge:       "dns-01",
			RenewBeforeDays: 30,
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     dir,
			},
		},
	}

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	mgr, err := NewManager(cfg, store, NewMockDNSProvider(), log.Nop())
	require.NoError(t, err)
	defer mgr.Stop()

	// Initial GetCertificate should fail as no cert is loaded yet
	_, err = mgr.GetCertificate(nil)
	assert.Error(t, err)

	// Set active cert
	certPEM, keyPEM := generateTestCertAndKey(t, "example.org")
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	err = mgr.setActiveCert(&tlsCert)
	require.NoError(t, err)

	// GetCertificate should now succeed
	active, err := mgr.GetCertificate(nil)
	require.NoError(t, err)
	assert.NotNil(t, active)

	// needsRenewal check
	assert.False(t, mgr.needsRenewal(&tlsCert))

	// Expiring cert (10 days remaining < 30 days threshold)
	expiringCert := &tls.Certificate{
		Leaf: &x509.Certificate{
			NotAfter: time.Now().Add(10 * 24 * time.Hour),
		},
	}
	assert.True(t, mgr.needsRenewal(expiringCert))
}

func TestProvideDNSProvider(t *testing.T) {
	// Disabled
	cfg := &config.Config{ACME: config.ACMEConfig{Enabled: false}}
	assert.Nil(t, ProvideDNSProvider(cfg))

	// Cloudflare
	cfg.ACME.Enabled = true
	cfg.ACME.Challenge = "dns-01"
	cfg.ACME.DNS.Provider = "cloudflare"
	cfg.ACME.DNS.CloudflareAPIToken = "tok"
	assert.IsType(t, &CloudflareProvider{}, ProvideDNSProvider(cfg))

	// RFC 2136
	cfg.ACME.DNS.Provider = "rfc2136"
	cfg.ACME.DNS.RFC2136Nameserver = "1.2.3.4:53"
	assert.IsType(t, &RFC2136Provider{}, ProvideDNSProvider(cfg))

	// Exec
	cfg.ACME.DNS.Provider = "exec"
	cfg.ACME.DNS.ExecScript = "/script.sh"
	assert.IsType(t, &ExecProvider{}, ProvideDNSProvider(cfg))

	// Mock
	cfg.ACME.DNS.Provider = "mock"
	assert.IsType(t, &MockDNSProvider{}, ProvideDNSProvider(cfg))
}

func TestManager_GetCertificate_OnDemandAndAuthorization(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:         true,
			Email:           "admin@example.org",
			Challenge:       "http-01",
			RenewBeforeDays: 30,
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     dir,
			},
		},
	}

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	mgr, err := NewManager(cfg, store, nil, log.Nop())
	require.NoError(t, err)
	defer mgr.Stop()

	// Store a certificate for a custom clinic domain
	customDomain := "clinicasaojose.com.br"
	certPEM, keyPEM := generateTestCertAndKey(t, customDomain)
	require.NoError(t, store.SaveCertificate(t.Context(), customDomain, certPEM, keyPEM))

	// Configure authorizer
	mgr.SetDomainAuthorizer(func(_ context.Context, domain string) (bool, error) {
		return domain == customDomain, nil
	})

	// GetCertificate with authorized custom domain found in store
	helloAuthorized := &tls.ClientHelloInfo{ServerName: customDomain}
	cert, err := mgr.GetCertificate(helloAuthorized)
	require.NoError(t, err)
	assert.NotNil(t, cert)

	// GetCertificate with unauthorized domain
	helloUnauthorized := &tls.ClientHelloInfo{ServerName: "unauthorized.com"}
	_, err = mgr.GetCertificate(helloUnauthorized)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized domain")
}

func TestManager_GetCertificate_DevelopmentMode_SelfSigned(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Mode:       "development",
		BaseDomain: "lv.test",
		ACME: config.ACMEConfig{
			Enabled: true,
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     dir,
			},
		},
	}

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	mgr, err := NewManager(cfg, store, nil, log.Nop())
	require.NoError(t, err)
	defer mgr.Stop()

	// Start in development mode skips ACME registration and generates self-signed fallback cert for lv.test
	err = mgr.Start(t.Context())
	require.NoError(t, err)

	// Fallback cert check for empty/nil ClientHello
	fallbackCert, err := mgr.GetCertificate(nil)
	require.NoError(t, err)
	assert.NotNil(t, fallbackCert)
	assert.NoError(t, fallbackCert.Leaf.VerifyHostname("lv.test"))

	// Configure authorizer for local clinic domain
	customDomain := "clinica1.local"
	mgr.SetDomainAuthorizer(func(_ context.Context, domain string) (bool, error) {
		return domain == customDomain, nil
	})

	// On-demand GetCertificate in dev generates self-signed certificate dynamically
	helloAuthorized := &tls.ClientHelloInfo{ServerName: customDomain}
	cert, err := mgr.GetCertificate(helloAuthorized)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.NoError(t, cert.Leaf.VerifyHostname(customDomain))
	assert.False(t, mgr.needsRenewal(cert))

	// Second request should hit memory cache
	cert2, err := mgr.GetCertificate(helloAuthorized)
	require.NoError(t, err)
	assert.Same(t, cert, cert2)

	// Unauthorized domain in dev is rejected
	helloUnauthorized := &tls.ClientHelloInfo{ServerName: "unknown.local"}
	_, err = mgr.GetCertificate(helloUnauthorized)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized domain")
}

func TestGenerateSelfSignedCert(t *testing.T) {
	// DNS domain
	cert, err := GenerateSelfSignedCert("clinica.local")
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Contains(t, cert.Leaf.DNSNames, "clinica.local")
	assert.NoError(t, cert.Leaf.VerifyHostname("clinica.local"))

	// IP address
	ipCert, err := GenerateSelfSignedCert("127.0.0.1")
	require.NoError(t, err)
	require.NotNil(t, ipCert)
	assert.NoError(t, ipCert.Leaf.VerifyHostname("127.0.0.1"))
}

func TestManager_GetCertificate_OnDemandDisabled(t *testing.T) {
	dir := t.TempDir()
	onDemandFalse := false
	cfg := &config.Config{
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:  true,
			Email:    "admin@example.org",
			OnDemand: &onDemandFalse,
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     dir,
			},
		},
	}

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	mgr, err := NewManager(cfg, store, nil, log.Nop())
	require.NoError(t, err)
	defer mgr.Stop()

	// Configure authorizer for custom domain
	customDomain := "clinica.org"
	mgr.SetDomainAuthorizer(func(_ context.Context, domain string) (bool, error) {
		return domain == customDomain, nil
	})

	// GetCertificate with on-demand disabled should fail and not attempt issuance
	hello := &tls.ClientHelloInfo{ServerName: customDomain}
	_, err = mgr.GetCertificate(hello)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "on-demand issuance is disabled")
}
