package kv

import (
	"context"
	"encoding/base64"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/config"
)

func TestBBoltStoreCRUDAndPrefix(t *testing.T) {
	s, err := OpenBBolt(filepath.Join(t.TempDir(), "kv.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	_, err = s.Get(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.Put(ctx, "urn:librevita:meta:a", []byte("1")))
	require.NoError(t, s.Put(ctx, "urn:librevita:meta:b", []byte("2")))
	require.NoError(t, s.Put(ctx, "urn:librevita:session:x", []byte("3")))

	got, err := s.Get(ctx, "urn:librevita:meta:a")
	require.NoError(t, err)
	assert.Equal(t, []byte("1"), got)

	created, err := s.PutIfAbsent(ctx, "urn:librevita:meta:a", []byte("other"))
	require.NoError(t, err)
	assert.False(t, created)

	created, err = s.PutIfAbsent(ctx, "urn:librevita:meta:c", []byte("3"))
	require.NoError(t, err)
	assert.True(t, created)

	batch, err := s.GetMany(ctx, []string{"urn:librevita:meta:a", "missing", "urn:librevita:meta:a"})
	require.NoError(t, err)
	assert.Equal(t, []byte("1"), batch["urn:librevita:meta:a"].Value)
	assert.ErrorIs(t, batch["missing"].Err, ErrNotFound)

	listed, err := s.ListPrefix(ctx, "urn:librevita:meta:")
	require.NoError(t, err)
	assert.Len(t, listed, 3)

	require.NoError(t, s.Delete(ctx, "urn:librevita:meta:a"))
	_, err = s.Get(ctx, "urn:librevita:meta:a")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestOpenRejectsVaultUnlessAllowed(t *testing.T) {
	cfg := config.KVConfig{Backend: config.BackendVault}
	_, err := Open(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only supported for the keystore")

	_, err = Open(cfg, AllowVault())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault address is required")
}

func TestOpenRejectsUnknownBackend(t *testing.T) {
	_, err := Open(config.KVConfig{Backend: "redis"})
	require.Error(t, err)
}

func TestAdapterConstructorsValidation(t *testing.T) {
	_, err := OpenNATS("", "bucket")
	assert.Error(t, err)
	_, err = OpenEtcd("", "/prefix")
	assert.Error(t, err)
	_, err = OpenVault("", "token", "secret", "librevita/keystore/")
	assert.Error(t, err)
	_, err = OpenVault("http://localhost:8200", "", "secret", "librevita/keystore/")
	assert.Error(t, err)
	_, err = OpenBBolt("")
	assert.Error(t, err)
}

func TestSanitizeNATSKeyRoundTrip(t *testing.T) {
	urn := "urn:librevita:patient:123/456.789"
	encoded := natsKey(urn)
	assert.Equal(t, "k_"+base64.RawURLEncoding.EncodeToString([]byte(urn)), encoded)
	got, ok := decodeNATSKey(encoded)
	assert.True(t, ok)
	assert.Equal(t, urn, got)
}

func TestIsVaultNotFound(t *testing.T) {
	assert.True(t, isVaultNotFound(api.ErrSecretNotFound))

	respErr := &api.ResponseError{StatusCode: http.StatusNotFound}
	assert.True(t, isVaultNotFound(respErr))

	rawErr := errors.New("URL GET http://localhost:8200: 404 secret not found")
	assert.True(t, isVaultNotFound(rawErr))

	otherErr := errors.New("500 internal server error")
	assert.False(t, isVaultNotFound(otherErr))
}
