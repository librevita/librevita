package keystore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/pkg/log"
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

	results, err = batch.GetDEKs(ctx, []string{urn})
	require.NoError(t, err)
	assert.ErrorIs(t, results[urn].Err, crypto.ErrKeyDestroyed)

	created, err = conditional.PutIfAbsent(ctx, urn, payload)
	assert.False(t, created)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
}

func TestNewKeyStoreFromConfigLifecycle(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		Keystore: config.KVConfig{
			Backend: "bbolt",
			BBolt: config.BBoltConfig{
				Path: "",
			},
		},
	}

	ks, err := NewKeyStoreFromConfig(cfg, lc, logger)
	require.NoError(t, err)
	assert.NotNil(t, ks)

	require.NoError(t, lc.Start(context.Background()))
	require.NoError(t, lc.Stop(context.Background()))
	assert.NotNil(t, Module)

	_, err = OpenBBolt("")
	assert.Error(t, err)
}
