package acme

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/log"
)

// Manager coordinates ACME client lifecycle, challenges, issuance, and renewal.
type Manager struct {
	cfg         *config.Config
	store       CertStore
	dnsProvider DNSProvider
	http01      *HTTP01Registry
	logger      log.Logger
	acmeClient  *acme.Client
	accountKey  crypto.Signer
	activeCert  atomic.Pointer[tls.Certificate]
	stopWorker  chan struct{}
}

// NewManager creates a new ACME Manager.
func NewManager(cfg *config.Config, store CertStore, dnsProvider DNSProvider, logger log.Logger) (*Manager, error) {
	if store == nil {
		dir := cfg.ACME.Storage.Dir
		if dir == "" {
			dataDir := cfg.DataDir
			if dataDir == "" {
				dataDir = "./data"
			}
			dir = filepath.Join(dataDir, "acme")
		}
		var err error
		store, err = NewFileCertStore(dir)
		if err != nil {
			return nil, err
		}
	}

	return &Manager{
		cfg:         cfg,
		store:       store,
		dnsProvider: dnsProvider,
		http01:      NewHTTP01Registry(),
		logger:      logger,
		stopWorker:  make(chan struct{}),
	}, nil
}

// HTTP01Registry returns the registry for HTTP-01 challenge routing.
func (m *Manager) HTTP01Registry() *HTTP01Registry {
	return m.http01
}

// GetCertificate implements tls.Config.GetCertificate for zero-downtime dynamic TLS handshakes.
func (m *Manager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := m.activeCert.Load()
	if cert == nil {
		return nil, errors.New("acme: no certificate available yet")
	}
	return cert, nil
}

// Start initializes the ACME account and loads or provisions certificates.
func (m *Manager) Start(ctx context.Context) error {
	if !m.cfg.ACME.Enabled {
		return nil
	}

	if err := m.initAccount(ctx); err != nil {
		return errors.Wrap(err, "acme: init account")
	}

	primaryDomain := m.primaryDomain()
	cachedCert, err := m.store.LoadCertificate(ctx, primaryDomain)
	if err == nil && cachedCert != nil {
		if err := m.setActiveCert(cachedCert); err == nil && !m.needsRenewal(cachedCert) {
			m.logger.InfoContext(ctx, "acme: loaded active certificate from cache",
				log.String("domain", primaryDomain),
			)
			m.startRenewalWorker()
			return nil
		}
	}

	m.logger.InfoContext(ctx, "acme: obtaining certificate from directory",
		log.String("directory", m.cfg.ACME.Directory),
		log.String("domain", primaryDomain),
	)
	if err := m.IssueCertificate(ctx); err != nil {
		m.logger.ErrorContext(ctx, "acme: initial certificate issuance failed", log.Error(err))
		// We still start the renewal worker so it retries periodically
	}

	m.startRenewalWorker()
	return nil
}

// Stop terminates background renewal.
func (m *Manager) Stop() {
	select {
	case <-m.stopWorker:
	default:
		close(m.stopWorker)
	}
}

func (m *Manager) primaryDomain() string {
	if len(m.cfg.ACME.Domains) > 0 {
		return m.cfg.ACME.Domains[0]
	}
	return m.cfg.BaseDomain
}

func (m *Manager) initAccount(ctx context.Context) error {
	key, err := m.store.LoadAccountKey(ctx)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		newKey, err := GeneratePrivateKey()
		if err != nil {
			return errors.Wrap(err, "acme: generate account key")
		}
		if err := m.store.SaveAccountKey(ctx, newKey); err != nil {
			return errors.Wrap(err, "acme: save account key")
		}
		key = newKey
	}
	m.accountKey = key

	m.acmeClient = &acme.Client{
		Key:          m.accountKey,
		DirectoryURL: m.cfg.ACME.Directory,
	}

	// Discover and register account
	acct := &acme.Account{
		Contact: []string{"mailto:" + m.cfg.ACME.Email},
	}
	_, err = m.acmeClient.Register(ctx, acct, acme.AcceptTOS)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		// Existing registration is acceptable
		m.logger.DebugContext(ctx, "acme: account registration status", log.Error(err))
	}
	return nil
}

// IssueCertificate coordinates the full ACME order authorization, challenge, and finalization.
func (m *Manager) IssueCertificate(ctx context.Context) error {
	domains := m.cfg.ACME.Domains
	if len(domains) == 0 {
		return errors.New("acme: no domains configured for certificate")
	}

	order, err := m.acmeClient.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return errors.Wrap(err, "acme: authorize order")
	}

	if err := m.fulfillAuthorizations(ctx, order.AuthzURLs); err != nil {
		return err
	}

	return m.finalizeOrder(ctx, order, domains)
}

func (m *Manager) fulfillAuthorizations(ctx context.Context, authzURLs []string) error {
	for _, authzURL := range authzURLs {
		authz, err := m.acmeClient.GetAuthorization(ctx, authzURL)
		if err != nil {
			return errors.Wrapf(err, "acme: get authz %s", authzURL)
		}
		if authz.Status == acme.StatusValid {
			continue
		}

		if err := m.solveChallenge(ctx, authz); err != nil {
			return err
		}

		if _, err := m.acmeClient.WaitAuthorization(ctx, authzURL); err != nil {
			return errors.Wrapf(err, "acme: wait authorization %s", authzURL)
		}
	}
	return nil
}

func (m *Manager) solveChallenge(ctx context.Context, authz *acme.Authorization) error {
	challengeType := m.cfg.ACME.Challenge
	var chal *acme.Challenge
	for _, c := range authz.Challenges {
		if c.Type == challengeType {
			chal = c
			break
		}
	}
	if chal == nil {
		return errors.Newf("acme: challenge %s not offered for domain %s", challengeType, authz.Identifier.Value)
	}

	if challengeType == config.ACMEChallengeDNS01 {
		return m.solveDNS01(ctx, authz.Identifier.Value, chal)
	}
	return m.solveHTTP01(ctx, chal)
}

func (m *Manager) solveDNS01(ctx context.Context, domain string, chal *acme.Challenge) error {
	if m.dnsProvider == nil {
		return errors.New("acme: no dns provider configured for dns-01 challenge")
	}
	recVal, err := m.acmeClient.DNS01ChallengeRecord(chal.Token)
	if err != nil {
		return errors.Wrap(err, "acme: compute dns-01 record")
	}

	m.logger.InfoContext(ctx, "acme: presenting dns-01 record",
		log.String("domain", domain),
		log.String("record_name", ChallengeRecordName(domain)),
	)
	if err := m.dnsProvider.Present(ctx, domain, recVal); err != nil {
		return errors.Wrap(err, "acme: dns provider present")
	}
	defer func() {
		_ = m.dnsProvider.CleanUp(ctx, domain, recVal)
	}()

	timeout := time.Duration(m.cfg.ACME.DNS.PropagationTimeoutSec) * time.Second
	_ = WaitForDNSPropagation(ctx, domain, recVal, timeout, nil, m.logger)

	if _, err := m.acmeClient.Accept(ctx, chal); err != nil {
		return errors.Wrap(err, "acme: accept dns-01 challenge")
	}
	return nil
}

func (m *Manager) solveHTTP01(ctx context.Context, chal *acme.Challenge) error {
	keyAuth, err := m.acmeClient.HTTP01ChallengeResponse(chal.Token)
	if err != nil {
		return errors.Wrap(err, "acme: compute http-01 response")
	}

	m.http01.Register(chal.Token, keyAuth)
	defer m.http01.Unregister(chal.Token)

	if _, err := m.acmeClient.Accept(ctx, chal); err != nil {
		return errors.Wrap(err, "acme: accept http-01 challenge")
	}
	return nil
}

func (m *Manager) finalizeOrder(ctx context.Context, order *acme.Order, domains []string) error {
	certKey, err := GeneratePrivateKey()
	if err != nil {
		return errors.Wrap(err, "acme: generate cert key")
	}

	csrReq := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, csrReq, certKey)
	if err != nil {
		return errors.Wrap(err, "acme: create csr")
	}

	derCerts, certURL, err := m.acmeClient.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return errors.Wrap(err, "acme: finalize order")
	}

	if len(derCerts) == 0 && certURL != "" {
		if _, err := m.acmeClient.WaitOrder(ctx, order.URI); err != nil {
			return errors.Wrap(err, "acme: wait order")
		}
		certs, err := m.acmeClient.FetchCert(ctx, certURL, true)
		if err != nil {
			return errors.Wrap(err, "acme: fetch cert")
		}
		derCerts = certs
	}

	return m.persistAndActivate(ctx, domains[0], derCerts, certKey)
}

func (m *Manager) persistAndActivate(ctx context.Context, primaryDomain string, derCerts [][]byte, certKey crypto.Signer) error {
	var certPEM bytes.Buffer
	for _, b := range derCerts {
		_ = pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: b})
	}
	keyPEM, err := encodePrivateKey(certKey)
	if err != nil {
		return err
	}

	if err := m.store.SaveCertificate(ctx, primaryDomain, certPEM.Bytes(), keyPEM); err != nil {
		m.logger.WarnContext(ctx, "acme: failed to save certificate to store", log.Error(err))
	}

	tlsCert, err := tls.X509KeyPair(certPEM.Bytes(), keyPEM)
	if err != nil {
		return errors.Wrap(err, "acme: build tls keypair")
	}

	if err := m.setActiveCert(&tlsCert); err != nil {
		return err
	}

	m.logger.InfoContext(ctx, "acme: certificate issued and activated",
		log.String("domain", primaryDomain),
	)
	return nil
}

func (m *Manager) setActiveCert(cert *tls.Certificate) error {
	if cert.Leaf == nil && len(cert.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return errors.Wrap(err, "acme: parse leaf cert")
		}
		cert.Leaf = leaf
	}
	m.activeCert.Store(cert)
	return nil
}

func (m *Manager) needsRenewal(cert *tls.Certificate) bool {
	if cert == nil || cert.Leaf == nil {
		return true
	}
	threshold := time.Duration(m.cfg.ACME.RenewBeforeDays) * 24 * time.Hour
	return time.Until(cert.Leaf.NotAfter) <= threshold
}

func (m *Manager) startRenewalWorker() {
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopWorker:
				return
			case <-ticker.C:
				cert := m.activeCert.Load()
				if m.needsRenewal(cert) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
					m.logger.InfoContext(ctx, "acme: starting scheduled certificate renewal")
					if err := m.IssueCertificate(ctx); err != nil {
						m.logger.ErrorContext(ctx, "acme: certificate renewal failed", log.Error(err))
					}
					cancel()
				}
			}
		}
	}()
}
