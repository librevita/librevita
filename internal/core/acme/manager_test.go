package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acmelib "golang.org/x/crypto/acme"

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

func TestManager_EdgeCasesAndHelpers(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	cfg := &config.Config{
		Mode:       "development",
		BaseDomain: "platform.test",
		ACME: config.ACMEConfig{
			Enabled: true,
			Email:   "dev@example.org",
			Domains: []string{"first.test", "second.test"},
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

	// HTTP01Registry
	assert.NotNil(t, mgr.HTTP01Registry())

	// primaryDomain
	assert.Equal(t, "first.test", mgr.primaryDomain())
	cfg.ACME.Domains = nil
	assert.Equal(t, "platform.test", mgr.primaryDomain())

	// normalizeHelloDomain
	assert.Equal(t, "example.org", normalizeHelloDomain("example.org:443"))
	assert.Equal(t, "example.org", normalizeHelloDomain("example.org."))
	assert.Equal(t, "example.org", normalizeHelloDomain("EXAMPLE.ORG"))

	// isDomainAuthorized
	assert.False(t, mgr.isDomainAuthorized(ctx, "any.org")) // nil authorizer

	mgr.SetDomainAuthorizer(func(_ context.Context, _ string) (bool, error) {
		return false, assert.AnError
	})
	assert.False(t, mgr.isDomainAuthorized(ctx, "any.org")) // authorizer error

	// setActiveCert invalid
	invalidCert := &tls.Certificate{Certificate: [][]byte{[]byte("bad-cert-der")}}
	assert.Error(t, mgr.setActiveCert(invalidCert))

	// needsRenewal nil leaf
	certNoLeaf := &tls.Certificate{Certificate: [][]byte{}}
	assert.False(t, mgr.needsRenewal(certNoLeaf))

	// Start in development mode provisions self-signed cert
	require.NoError(t, mgr.Start(ctx))
	assert.NotNil(t, mgr.activeCert.Load())

	// Start when disabled
	cfgDisabled := &config.Config{ACME: config.ACMEConfig{Enabled: false}}
	mgrDisabled, err := NewManager(cfgDisabled, store, nil, log.Nop())
	require.NoError(t, err)
	require.NoError(t, mgrDisabled.Start(ctx))

	// Stop idempotence
	mgr.Stop()
	mgr.Stop() // should not panic
}

func newMockACMEServer(t *testing.T, certPEM []byte) *httptest.Server {
	t.Helper()
	var s *httptest.Server
	var authzSolved atomic.Bool
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce-12345")
		switch r.URL.Path {
		case "/directory":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"newNonce":   s.URL + "/nonce",
				"newAccount": s.URL + "/new-account",
				"newOrder":   s.URL + "/new-order",
			})
		case "/nonce":
			w.WriteHeader(http.StatusOK)
		case "/new-account":
			w.Header().Set("Location", s.URL+"/account/1")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"valid"}`))
		case "/new-order":
			authzSolved.Store(false)
			w.Header().Set("Location", s.URL+"/order/1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":         "ready",
				"authorizations": []string{s.URL + "/authz/1"},
				"finalize":       s.URL + "/finalize",
				"certificate":    s.URL + "/cert",
			})
		case "/authz/1":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := "pending"
			if authzSolved.Load() {
				status = "valid"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": status,
				"identifier": map[string]string{
					"type":  "dns",
					"value": "example.org",
				},
				"challenges": []map[string]string{
					{
						"type":  "http-01",
						"url":   s.URL + "/chal/http",
						"token": "test-token",
					},
					{
						"type":  "dns-01",
						"url":   s.URL + "/chal/dns",
						"token": "test-token",
					},
				},
			})
		case "/chal/http", "/chal/dns":
			authzSolved.Store(true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "valid",
			})
		case "/finalize":
			var jws struct {
				Payload string `json:"payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&jws); err == nil && jws.Payload != "" {
				rawPayload, _ := base64.RawURLEncoding.DecodeString(jws.Payload)
				var body struct {
					CSR string `json:"csr"`
				}
				if err := json.Unmarshal(rawPayload, &body); err == nil && body.CSR != "" {
					csrDER, _ := base64.RawURLEncoding.DecodeString(body.CSR)
					if csr, err := x509.ParseCertificateRequest(csrDER); err == nil {
						signer, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
						template := x509.Certificate{
							SerialNumber: big.NewInt(1),
							Subject:      csr.Subject,
							DNSNames:     csr.DNSNames,
							NotBefore:    time.Now().Add(-time.Hour),
							NotAfter:     time.Now().Add(30 * 24 * time.Hour),
						}
						der, _ := x509.CreateCertificate(rand.Reader, &template, &template, csr.PublicKey, signer)
						certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":      "valid",
				"certificate": s.URL + "/cert",
			})
		case "/cert":
			w.Header().Set("Content-Type", "application/pem-certificate-chain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(certPEM)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestManager_ProductionStartupWithCachedCertAndRenewal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	certPEM, keyPEM := generateTestCertAndKey(t, "example.org")
	server := newMockACMEServer(t, certPEM)

	cfg := &config.Config{
		Mode:       "production",
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:         true,
			Email:           "admin@example.org",
			Directory:       server.URL + "/directory",
			Domains:         []string{"example.org"},
			RenewBeforeDays: 30,
			Storage: config.ACMEStorageConfig{
				Backend: "file",
				Dir:     dir,
			},
		},
	}

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	// Pre-generate and cache a valid non-expiring certificate in store
	require.NoError(t, store.SaveCertificate(ctx, "example.org", certPEM, keyPEM))

	mgr, err := NewManager(cfg, store, NewMockDNSProvider(), log.Nop())
	require.NoError(t, err)
	defer mgr.Stop()

	// Start in production mode:
	// Should initialize account with mock ACME server, load cached cert, and start renewal worker
	err = mgr.Start(ctx)
	require.NoError(t, err)
	assert.NotNil(t, mgr.activeCert.Load())

	// Test renewExpiringCerts:
	// When cert doesn't need renewal:
	mgr.renewExpiringCerts()

	// When on-demand is disabled:
	onDemandFalse := false
	mgr.cfg.ACME.OnDemand = &onDemandFalse
	mgr.renewExpiringCerts()

	// When in dev mode:
	mgr.cfg.Mode = "development"
	mgr.renewExpiringCerts()
}

func TestManager_ChallengeErrors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:   true,
			Challenge: "dns-01",
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

	// ObtainCertificateHTTP01 with nil acmeClient
	_, err = mgr.ObtainCertificateHTTP01(ctx, "example.org")
	assert.Error(t, err)

	// IssueCertificate with no domains
	cfg.ACME.Domains = nil
	err = mgr.IssueCertificate(ctx)
	assert.Error(t, err)

	// solveDNS01 with nil dnsProvider
	chal := &acmelib.Challenge{Type: "dns-01", Token: "tok"}
	err = mgr.solveDNS01(ctx, "example.org", chal)
	assert.Error(t, err)

	// solveChallenge with challenge not offered
	authzNoMatch := &acmelib.Authorization{
		Identifier: acmelib.AuthzID{Value: "example.org"},
		Challenges: []*acmelib.Challenge{{Type: "tls-alpn-01"}},
	}
	err = mgr.solveChallenge(ctx, authzNoMatch)
	assert.Error(t, err)
}

func TestManager_FullIssuanceAndOrderFinalization(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	certPEM, _ := generateTestCertAndKey(t, "example.org")
	server := newMockACMEServer(t, certPEM)

	cfg := &config.Config{
		Mode:       "production",
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:   true,
			Email:     "admin@example.org",
			Challenge: "dns-01",
			Directory: server.URL + "/directory",
			Domains:   []string{"example.org"},
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

	// Initialize account
	require.NoError(t, mgr.initAccount(ctx))

	// Issue certificate (calls AuthorizeOrder -> fulfillAuthorizations -> finalizeOrder -> persistAndBuildCert -> setActiveCert)
	err = mgr.IssueCertificate(ctx)
	require.NoError(t, err)
	assert.NotNil(t, mgr.activeCert.Load())

	// Verify cert was saved to store and matches
	loaded, err := store.LoadCertificate(ctx, "example.org")
	require.NoError(t, err)
	assert.NotNil(t, loaded)
}

func TestManager_NewManager_DefaultsAndErrors(t *testing.T) {
	// Store nil with empty Dir and empty DataDir
	cfg := &config.Config{}
	mgr, err := NewManager(cfg, nil, nil, log.Nop())
	require.NoError(t, err)
	assert.NotNil(t, mgr)

	// Store nil with invalid dir path
	cfgBad := &config.Config{
		ACME: config.ACMEConfig{
			Storage: config.ACMEStorageConfig{
				Dir: "/dev/null/cannot-mkdir",
			},
		},
	}
	_, err = NewManager(cfgBad, nil, nil, log.Nop())
	assert.Error(t, err)
}

func TestManager_ObtainCertificateHTTP01_Success(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	certPEM, _ := generateTestCertAndKey(t, "example.org")
	server := newMockACMEServer(t, certPEM)

	cfg := &config.Config{
		Mode:       "production",
		BaseDomain: "example.org",
		ACME: config.ACMEConfig{
			Enabled:   true,
			Email:     "admin@example.org",
			Directory: server.URL + "/directory",
			Domains:   []string{"example.org"},
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

	require.NoError(t, mgr.initAccount(ctx))

	cert, err := mgr.ObtainCertificateHTTP01(ctx, "example.org")
	require.NoError(t, err)
	assert.NotNil(t, cert)

	// Test obtainOnDemand in production mode
	onDemandCert, err := mgr.obtainOnDemand("example.org")
	require.NoError(t, err)
	assert.NotNil(t, onDemandCert)

	// Second call should hit in-memory cache
	cachedCert, err := mgr.obtainOnDemand("example.org")
	require.NoError(t, err)
	assert.Same(t, onDemandCert, cachedCert)

	// Test renewExpiringCerts with expiring cert in cache
	expiringCert := &tls.Certificate{
		Leaf: &x509.Certificate{
			NotAfter: time.Now().Add(time.Hour),
		},
	}
	mgr.certCache.Store("expiring.org", expiringCert)
	mgr.renewExpiringCerts()
}
