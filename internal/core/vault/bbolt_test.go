package vault

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

func TestBBoltVaultDirectOperations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "keys.db")

	v, err := NewBBoltVault(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	ctx := context.Background()
	patientURN := "urn:librevita:patient:018f7654-3210-7000-8000-000000000001"
	encDEK := []byte("encrypted-dek-payload-32-bytes!!")

	// Get before Put should return crypto.ErrKeyNotFound
	_, err = v.GetDEK(ctx, patientURN)
	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)

	// Put DEK
	err = v.PutDEK(ctx, patientURN, encDEK)
	require.NoError(t, err)

	// Get DEK
	got, err := v.GetDEK(ctx, patientURN)
	require.NoError(t, err)
	assert.Equal(t, encDEK, got)

	// Delete DEK (Crypto-Shredding)
	err = v.DeleteDEK(ctx, patientURN)
	require.NoError(t, err)

	// Get after Delete should return the terminal shredding error.
	_, err = v.GetDEK(ctx, patientURN)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
}

func TestBBoltVaultBatchAndCreateIfAbsent(t *testing.T) {
	dir := t.TempDir()
	v, err := NewBBoltVault(filepath.Join(dir, "keys.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	ctx := context.Background()
	first := "urn:librevita:clinic:first"
	second := "urn:librevita:clinic:second"
	payload := []byte("wrapped-dek")

	created, err := v.PutIfAbsent(ctx, first, payload)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = v.PutIfAbsent(ctx, first, []byte("other"))
	require.NoError(t, err)
	assert.False(t, created)
	got, err := v.GetDEK(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	batch, err := v.GetDEKs(ctx, []string{first, second, first})
	require.NoError(t, err)
	assert.Equal(t, payload, batch[first].EncryptedDEK)
	assert.ErrorIs(t, batch[second].Err, crypto.ErrKeyNotFound)

	require.NoError(t, v.DeleteDEK(ctx, first))
	created, err = v.PutIfAbsent(ctx, first, payload)
	assert.False(t, created)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
	batch, err = v.GetDEKs(ctx, []string{first})
	require.NoError(t, err)
	assert.ErrorIs(t, batch[first].Err, crypto.ErrKeyDestroyed)
}
