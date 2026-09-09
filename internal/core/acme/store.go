// Package acme provides Let's Encrypt / ACME client certificate management
// with support for HTTP-01 and DNS-01 challenges.
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"

	"librevita.org/internal/core/kv"
	"librevita.org/pkg/errors"
)

var (
	// ErrNotFound indicates that the requested key or certificate was not found.
	ErrNotFound = errors.New("acme: not found in store")
)

// CertStore defines the persistence interface for ACME account keys and certificates.
type CertStore interface {
	LoadAccountKey(ctx context.Context) (crypto.Signer, error)
	SaveAccountKey(ctx context.Context, key crypto.Signer) error
	LoadCertificate(ctx context.Context, domain string) (*tls.Certificate, error)
	SaveCertificate(ctx context.Context, domain string, certPEM, keyPEM []byte) error
}

// FileCertStore persists ACME artifacts on the local filesystem with 0600 permissions.
type FileCertStore struct {
	dir string
}

// NewFileCertStore creates a new FileCertStore rooted at dir.
func NewFileCertStore(dir string) (*FileCertStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.Wrapf(err, "acme: failed to create store directory %q", dir)
	}
	return &FileCertStore{dir: dir}, nil
}

func (s *FileCertStore) accountKeyPath() string {
	return filepath.Join(s.dir, "account.key")
}

func (s *FileCertStore) certPath(domain string) string {
	safe := sanitizeDomain(domain)
	return filepath.Join(s.dir, safe+".crt")
}

func (s *FileCertStore) keyPath(domain string) string {
	safe := sanitizeDomain(domain)
	return filepath.Join(s.dir, safe+".key")
}

// LoadAccountKey reads the account private key from disk.
func (s *FileCertStore) LoadAccountKey(ctx context.Context) (crypto.Signer, error) {
	data, err := os.ReadFile(s.accountKeyPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme: read account key")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("acme: invalid PEM data for account key")
	}
	return parsePrivateKey(block.Bytes)
}

// SaveAccountKey writes the account private key with 0600 permissions.
func (s *FileCertStore) SaveAccountKey(ctx context.Context, key crypto.Signer) error {
	encoded, err := encodePrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(s.accountKeyPath(), encoded, 0o600)
}

// LoadCertificate reads the domain's certificate and private key from disk.
func (s *FileCertStore) LoadCertificate(ctx context.Context, domain string) (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(s.certPath(domain))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme: read certificate")
	}
	keyPEM, err := os.ReadFile(s.keyPath(domain))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme: read certificate key")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "acme: parse key pair")
	}
	return &cert, nil
}

// SaveCertificate writes the certificate and private key with 0600 permissions.
func (s *FileCertStore) SaveCertificate(ctx context.Context, domain string, certPEM, keyPEM []byte) error {
	if err := os.WriteFile(s.certPath(domain), certPEM, 0o600); err != nil {
		return errors.Wrap(err, "acme: write certificate")
	}
	if err := os.WriteFile(s.keyPath(domain), keyPEM, 0o600); err != nil {
		return errors.Wrap(err, "acme: write certificate key")
	}
	return nil
}

// KVCertStore persists ACME artifacts inside a kv.Store backend.
type KVCertStore struct {
	store kv.Store
}

// NewKVCertStore creates a CertStore backed by a kv.Store.
func NewKVCertStore(store kv.Store) *KVCertStore {
	return &KVCertStore{store: store}
}

func (s *KVCertStore) LoadAccountKey(ctx context.Context) (crypto.Signer, error) {
	data, err := s.store.Get(ctx, "urn:librevita:acme:account_key")
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme kv: get account key")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("acme kv: invalid PEM in account key")
	}
	return parsePrivateKey(block.Bytes)
}

func (s *KVCertStore) SaveAccountKey(ctx context.Context, key crypto.Signer) error {
	encoded, err := encodePrivateKey(key)
	if err != nil {
		return err
	}
	return s.store.Put(ctx, "urn:librevita:acme:account_key", encoded)
}

func (s *KVCertStore) LoadCertificate(ctx context.Context, domain string) (*tls.Certificate, error) {
	safe := sanitizeDomain(domain)
	certPEM, err := s.store.Get(ctx, "urn:librevita:acme:cert:"+safe)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme kv: get cert")
	}
	keyPEM, err := s.store.Get(ctx, "urn:librevita:acme:key:"+safe)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "acme kv: get key")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "acme kv: parse key pair")
	}
	return &cert, nil
}

func (s *KVCertStore) SaveCertificate(ctx context.Context, domain string, certPEM, keyPEM []byte) error {
	safe := sanitizeDomain(domain)
	if err := s.store.Put(ctx, "urn:librevita:acme:cert:"+safe, certPEM); err != nil {
		return errors.Wrap(err, "acme kv: put cert")
	}
	if err := s.store.Put(ctx, "urn:librevita:acme:key:"+safe, keyPEM); err != nil {
		return errors.Wrap(err, "acme kv: put key")
	}
	return nil
}

func sanitizeDomain(domain string) string {
	s := strings.TrimSpace(domain)
	s = strings.ReplaceAll(s, "*", "wildcard")
	return strings.ReplaceAll(s, "/", "_")
}

func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("acme: unsupported private key type")
}

func encodePrivateKey(key crypto.Signer) ([]byte, error) {
	var der []byte
	var blockType string
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, errors.Wrap(err, "acme: marshal ecdsa key")
		}
		der = b
		blockType = "EC PRIVATE KEY"
	default:
		b, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, errors.Wrap(err, "acme: marshal pkcs8 key")
		}
		der = b
		blockType = "PRIVATE KEY"
	}
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), nil
}

// GeneratePrivateKey generates a secure P-256 ECDSA key.
func GeneratePrivateKey() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}
