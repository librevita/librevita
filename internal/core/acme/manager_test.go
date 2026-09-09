package acme

import (
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
