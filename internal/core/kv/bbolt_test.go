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

func TestOpenBBoltBackend(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "open.db")
	cfg := config.KVConfig{
		Backend: config.BackendBBolt,
		BBolt:   config.BBoltConfig{Path: tmpPath},
	}
	s, err := Open(cfg)
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "k1", []byte("v1")))
	v, err := s.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)
}

func TestVaultStoreHelpers(t *testing.T) {
	v, err := OpenVault("http://localhost:8200", "token123", "", "custom/prefix")
	require.NoError(t, err)
	assert.Equal(t, "custom/prefix/", v.prefix)
	assert.Equal(t, "secret", v.mount)
	assert.NotEmpty(t, v.path("urn:librevita:123"))
	assert.NoError(t, v.Close())
}

func TestBBoltDelete(t *testing.T) {
	s, err := OpenBBolt(filepath.Join(t.TempDir(), "delete.db"))
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	require.NoError(t, s.Put(ctx, "key-to-delete", []byte("sensitive-payload")))
	require.NoError(t, s.Delete(ctx, "key-to-delete"))

	// After delete, key is not found
	_, err = s.Get(ctx, "key-to-delete")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestEtcdHelpersAndNATSKeyInvalid(t *testing.T) {
	// 1. Etcd helpers
	etcdStore, err := OpenEtcd("localhost:2379", "my-prefix")
	require.NoError(t, err)
	assert.Equal(t, "my-prefix/test-key", etcdStore.key("test-key"))
	require.NoError(t, etcdStore.Close())

	// 2. DecodeNATSKey invalid formats
	_, ok := decodeNATSKey("no_prefix")
	assert.False(t, ok)

	_, ok = decodeNATSKey("k_!invalid_base64!")
	assert.False(t, ok)
}

func TestOpenBackendValidation(t *testing.T) {
	// NATS without url
	_, err := Open(config.KVConfig{Backend: config.BackendNATS})
	assert.Error(t, err)

	// Etcd without endpoints
	_, err = Open(config.KVConfig{Backend: config.BackendEtcd})
	assert.Error(t, err)

	// Vault without address (when allowed)
	_, err = Open(config.KVConfig{Backend: config.BackendVault}, AllowVault())
	assert.Error(t, err)
}

func TestBytesClone(t *testing.T) {
	orig := []byte("hello world")
	cloned := bytesClone(orig)
	assert.Equal(t, orig, cloned)
	cloned[0] = 'H'
	assert.NotEqual(t, orig[0], cloned[0])
}

func TestBBoltStoreCanceledContextAndEdgeCases(t *testing.T) {
	s, err := OpenBBolt(filepath.Join(t.TempDir(), "cancel.db"))
	require.NoError(t, err)
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.Get(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = s.GetMany(ctx, []string{"key"})
	assert.ErrorIs(t, err, context.Canceled)

	err = s.Put(ctx, "key", []byte("val"))
	assert.ErrorIs(t, err, context.Canceled)

	_, err = s.PutIfAbsent(ctx, "key", []byte("val"))
	assert.ErrorIs(t, err, context.Canceled)

	err = s.Delete(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	_, err = s.ListPrefix(ctx, "key")
	assert.ErrorIs(t, err, context.Canceled)

	// List non-existent prefix returns empty slice
	emptyEntries, err := s.ListPrefix(context.Background(), "missing_prefix:")
	require.NoError(t, err)
	assert.Empty(t, emptyEntries)
}

func TestBatchGetWithWorkersEmptyKeys(t *testing.T) {
	results, err := batchGetWithWorkers(context.Background(), nil, 4, func(context.Context, string) ([]byte, error) {
		t.Fatal("should not call get for empty keys")
		return nil, nil
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestBatchGetWithWorkersDefaultWorkers(t *testing.T) {
	// Pass workers=0 to hit the "workers <= 0" branch that defaults to 8
	results, err := batchGetWithWorkers(context.Background(), []string{"a"}, 0, func(_ context.Context, key string) ([]byte, error) {
		return []byte("val-" + key), nil
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("val-a"), results["a"].Value)
}

func TestUniqueKeysDedup(t *testing.T) {
	got := uniqueKeys([]string{"x", "y", "x", "z", "y"})
	assert.Equal(t, []string{"x", "y", "z"}, got)
}

func TestVaultListKeysHelper(t *testing.T) {
	// nil secret
	assert.Nil(t, vaultListKeys(nil))

	// secret with nil data
	assert.Nil(t, vaultListKeys(&api.Secret{}))

	// secret with wrong type for keys
	assert.Nil(t, vaultListKeys(&api.Secret{Data: map[string]interface{}{"keys": "not-a-slice"}}))

	// secret with empty entries in keys
	names := vaultListKeys(&api.Secret{Data: map[string]interface{}{
		"keys": []interface{}{"key1", "", "key2", 42},
	}})
	assert.Equal(t, []string{"key1", "key2"}, names)
}

func TestOpenEtcdPrefixSuffix(t *testing.T) {
	// Prefix without trailing slash gets one added
	store, err := OpenEtcd("localhost:2379", "no-slash")
	require.NoError(t, err)
	assert.Equal(t, "no-slash/", store.prefix)
	_ = store.Close()

	// Prefix with trailing slash stays the same
	store2, err := OpenEtcd("localhost:2379", "has-slash/")
	require.NoError(t, err)
	assert.Equal(t, "has-slash/", store2.prefix)
	_ = store2.Close()
}

func TestOpenNATSValidation(t *testing.T) {
	_, err := OpenNATS("nats://localhost:4222", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

func TestOpenEtcdValidation(t *testing.T) {
	_, err := OpenEtcd("localhost:2379", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prefix is required")
}
