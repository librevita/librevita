package meta

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/log"
)

func TestRepositoryRoundTrip(t *testing.T) {
	store, err := kv.OpenBBolt(filepath.Join(t.TempDir(), "meta.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	repo := NewRepository(store)
	ctx := context.Background()

	_, err = repo.Get(ctx, "flag")
	assert.ErrorIs(t, err, kv.ErrNotFound)

	require.NoError(t, repo.Put(ctx, "flag", "1"))
	got, err := repo.Get(ctx, "flag")
	require.NoError(t, err)
	assert.Equal(t, "1", got)

	require.NoError(t, repo.Delete(ctx, "flag"))
	_, err = repo.Get(ctx, "flag")
	assert.ErrorIs(t, err, kv.ErrNotFound)
}

func TestNewRepositoryFromConfigLifecycle(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	logger := log.Nop()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		Meta: config.KVConfig{
			Backend: "bbolt",
			BBolt: config.BBoltConfig{
				Path: "",
			},
		},
	}

	repo, err := NewRepositoryFromConfig(cfg, lc, logger)
	require.NoError(t, err)
	assert.NotNil(t, repo)

	require.NoError(t, lc.Start(context.Background()))
	require.NoError(t, lc.Stop(context.Background()))
	assert.NotNil(t, Module)
}
