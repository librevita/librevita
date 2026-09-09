package kv

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/config"
)

func TestOpen_Backends(t *testing.T) {
	// 1. Unsupported backend
	_, err := Open(config.KVConfig{Backend: "redis"})
	assert.Error(t, err)

	// 2. BBolt
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := Open(config.KVConfig{
		Backend: config.BackendBBolt,
		BBolt:   config.BBoltConfig{Path: dbPath},
	})
	require.NoError(t, err)
	assert.NoError(t, store.Close())

	// 3. Vault without AllowVault
	_, err = Open(config.KVConfig{
		Backend: config.BackendVault,
		Vault:   config.VaultBackendConfig{Address: "http://localhost:8200", Token: "token"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only supported for the keystore")

	// 4. Vault with AllowVault
	store, err = Open(config.KVConfig{
		Backend: config.BackendVault,
		Vault:   config.VaultBackendConfig{Address: "http://localhost:8200", Token: "token"},
	}, AllowVault())
	require.NoError(t, err)
	assert.NoError(t, store.Close())
}
