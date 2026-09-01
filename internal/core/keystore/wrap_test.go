package keystore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

func TestBBoltKeyStoreShreddingAndCreateIfAbsent(t *testing.T) {
	ks, err := OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ks.Close() })

	ctx := context.Background()
	urn := "urn:librevita:clinic:first"
	payload := []byte("wrapped-dek")

	_, err = ks.GetDEK(ctx, urn)
	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)

	require.NoError(t, ks.PutDEK(ctx, urn, payload))
	got, err := ks.GetDEK(ctx, urn)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	conditional, ok := ks.(crypto.ConditionalKeyStore)
	require.True(t, ok)
	created, err := conditional.PutIfAbsent(ctx, urn, []byte("other"))
	require.NoError(t, err)
	assert.False(t, created)

	batch, ok := ks.(crypto.BatchKeyStore)
	require.True(t, ok)
	results, err := batch.GetDEKs(ctx, []string{urn, "urn:librevita:clinic:missing"})
	require.NoError(t, err)
	assert.Equal(t, payload, results[urn].EncryptedDEK)
	assert.ErrorIs(t, results["urn:librevita:clinic:missing"].Err, crypto.ErrKeyNotFound)

	require.NoError(t, ks.DeleteDEK(ctx, urn))
	_, err = ks.GetDEK(ctx, urn)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
	created, err = conditional.PutIfAbsent(ctx, urn, payload)
	assert.False(t, created)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
}
