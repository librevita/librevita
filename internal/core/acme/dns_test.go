package acme

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/pkg/log"
)

func TestChallengeRecordName(t *testing.T) {
	assert.Equal(t, "_acme-challenge.example.org", ChallengeRecordName("example.org"))
	assert.Equal(t, "_acme-challenge.example.org", ChallengeRecordName("*.example.org"))
	assert.Equal(t, "_acme-challenge.sub.example.org", ChallengeRecordName("sub.example.org"))
}

func TestWaitForDNSPropagation(t *testing.T) {
	ctx := context.Background()
	logger := log.Nop()

	// Immediate success
	mockLookupSuccess := func(ctx context.Context, name string) ([]string, error) {
		return []string{"digest123"}, nil
	}
	err := WaitForDNSPropagation(ctx, "example.org", "digest123", 5*time.Second, mockLookupSuccess, logger)
	require.NoError(t, err)

	// Context canceled
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	mockLookupNever := func(ctx context.Context, name string) ([]string, error) {
		return []string{"other"}, nil
	}
	err = WaitForDNSPropagation(canceledCtx, "example.org", "digest123", 5*time.Second, mockLookupNever, logger)
	assert.Error(t, err)
}

func TestMockDNSProvider(t *testing.T) {
	ctx := context.Background()
	p := NewMockDNSProvider()

	require.NoError(t, p.Present(ctx, "example.org", "rec1"))
	val, ok := p.GetRecord("example.org")
	assert.True(t, ok)
	assert.Equal(t, "rec1", val)

	require.NoError(t, p.CleanUp(ctx, "example.org", "rec1"))
	_, ok = p.GetRecord("example.org")
	assert.False(t, ok)
}

func TestDefaultResolverLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _ = DefaultResolverLookup(ctx, "invalid.local.test")
}
