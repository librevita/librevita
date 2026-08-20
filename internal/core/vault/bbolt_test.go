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

	// Get after Delete should return crypto.ErrKeyNotFound
	_, err = v.GetDEK(ctx, patientURN)
	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)
}
