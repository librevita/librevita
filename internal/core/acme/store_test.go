package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/kv"
)

func generateTestCertAndKey(t *testing.T, domain string) ([]byte, []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		DNSNames:     []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

func TestFileCertStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	// Not found checks
	_, err = store.LoadAccountKey(ctx)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = store.LoadCertificate(ctx, "example.org")
	assert.ErrorIs(t, err, ErrNotFound)

	// Account key save and load
	key, err := GeneratePrivateKey()
	require.NoError(t, err)

	err = store.SaveAccountKey(ctx, key)
	require.NoError(t, err)

	// Verify permissions (0600)
	info, err := os.Stat(filepath.Join(dir, "account.key"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	loadedKey, err := store.LoadAccountKey(ctx)
	require.NoError(t, err)
	assert.NotNil(t, loadedKey)

	// Certificate save and load
	certPEM, keyPEM := generateTestCertAndKey(t, "example.org")
	err = store.SaveCertificate(ctx, "example.org", certPEM, keyPEM)
	require.NoError(t, err)

	keyInfo, err := os.Stat(filepath.Join(dir, "example.org.key"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())

	cert, err := store.LoadCertificate(ctx, "example.org")
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.NotEmpty(t, cert.Certificate)

	// Wildcard domain sanitization
	wildCertPEM, wildKeyPEM := generateTestCertAndKey(t, "*.example.org")
	err = store.SaveCertificate(ctx, "*.example.org", wildCertPEM, wildKeyPEM)
	require.NoError(t, err)

	loadedWild, err := store.LoadCertificate(ctx, "*.example.org")
	require.NoError(t, err)
	assert.NotNil(t, loadedWild)
}

func TestKVCertStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bboltStore, err := kv.OpenBBolt(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer func() { _ = bboltStore.Close() }()

	store := NewKVCertStore(bboltStore)

	// Not found
	_, err = store.LoadAccountKey(ctx)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = store.LoadCertificate(ctx, "example.org")
	assert.ErrorIs(t, err, ErrNotFound)

	// Save and load account key
	key, err := GeneratePrivateKey()
	require.NoError(t, err)

	err = store.SaveAccountKey(ctx, key)
	require.NoError(t, err)

	loadedKey, err := store.LoadAccountKey(ctx)
	require.NoError(t, err)
	assert.NotNil(t, loadedKey)

	// Save and load certificate
	certPEM, keyPEM := generateTestCertAndKey(t, "example.org")
	err = store.SaveCertificate(ctx, "example.org", certPEM, keyPEM)
	require.NoError(t, err)

	loadedCert, err := store.LoadCertificate(ctx, "example.org")
	require.NoError(t, err)
	assert.NotNil(t, loadedCert)

	// Corrupted PEM in KV store
	require.NoError(t, bboltStore.Put(ctx, "urn:librevita:acme:account_key", []byte("not-pem-data")))
	_, err = store.LoadAccountKey(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PEM")

	require.NoError(t, bboltStore.Put(ctx, "urn:librevita:acme:cert:corrupt.org", []byte("invalid-cert")))
	require.NoError(t, bboltStore.Put(ctx, "urn:librevita:acme:key:corrupt.org", []byte("invalid-key")))
	_, err = store.LoadCertificate(ctx, "corrupt.org")
	assert.Error(t, err)
}

func TestStore_RSAAndPKCSKeys(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewFileCertStore(dir)
	require.NoError(t, err)

	// RSA key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	err = store.SaveAccountKey(ctx, rsaKey)
	require.NoError(t, err)

	loadedKey, err := store.LoadAccountKey(ctx)
	require.NoError(t, err)
	assert.NotNil(t, loadedKey)

	// Corrupted account key file on disk
	err = os.WriteFile(filepath.Join(dir, "account.key"), []byte("invalid-data"), 0o600)
	require.NoError(t, err)
	_, err = store.LoadAccountKey(ctx)
	assert.Error(t, err)

	// Invalid cert/key PEM on disk
	err = os.WriteFile(filepath.Join(dir, "bad.org.crt"), []byte("bad-cert"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "bad.org.key"), []byte("bad-key"), 0o600)
	require.NoError(t, err)
	_, err = store.LoadCertificate(ctx, "bad.org")
	assert.Error(t, err)

	// sanitizeDomain checks
	assert.Equal(t, "wildcard.example.org", sanitizeDomain("*.example.org"))
	assert.Equal(t, "sub_domain", sanitizeDomain("sub/domain"))

	// parsePrivateKey unsupported type
	_, err = parsePrivateKey([]byte{0x01, 0x02})
	assert.Error(t, err)

	// NewFileCertStore invalid path
	_, err = NewFileCertStore("/dev/null/forbidden")
	assert.Error(t, err)
}
