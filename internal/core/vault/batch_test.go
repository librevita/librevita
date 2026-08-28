package vault

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

func TestBatchGetWithWorkersDeduplicatesAndPreservesItemErrors(t *testing.T) {
	var calls atomic.Int32
	results, err := batchGetWithWorkers(context.Background(),
		[]string{"a", "b", "a"},
		2,
		func(_ context.Context, urn string) ([]byte, error) {
			calls.Add(1)
			if urn == "b" {
				return nil, crypto.ErrKeyNotFound
			}
			return []byte("wrapped-" + urn), nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, []byte("wrapped-a"), results["a"].EncryptedDEK)
	assert.ErrorIs(t, results["b"].Err, crypto.ErrKeyNotFound)
}

func TestBatchGetWithWorkersHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := batchGetWithWorkers(ctx, []string{"a"}, 1, func(context.Context, string) ([]byte, error) {
		return nil, errors.New("must not run")
	})
	assert.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err = batchGetWithWorkers(ctx, []string{"a"}, 1, func(context.Context, string) ([]byte, error) {
		return []byte("wrapped"), nil
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
