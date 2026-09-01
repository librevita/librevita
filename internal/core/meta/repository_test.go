package meta

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/kv"
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
