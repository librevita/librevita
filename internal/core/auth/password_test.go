package auth

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPasswordAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	require.NoError(t, err)
	assert.True(t, ok, "VerifyPassword rejected the correct password")

	ok, err = VerifyPassword(hash, "wrong password")
	require.NoError(t, err)
	assert.False(t, ok, "VerifyPassword accepted a wrong password")
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, hash := range []string{"", "not-a-phc-hash", "$argon2id$v=19$m=8,t=3,p=2$AA$AA", "$argon2i$v=19$m=65536,t=3,p=2$AA$AA"} {
		_, err := VerifyPassword(hash, "x")
		assert.Error(t, err, "VerifyPassword(%q) should fail", hash)
	}
}

func TestHashesAreUnique(t *testing.T) {
	a, err := HashPassword("same password")
	require.NoError(t, err)
	b, err := HashPassword("same password")
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "two hashes of the same password must differ (salt)")
}

func TestConcurrentHashesRespectSemaphore(t *testing.T) {
	SetMaxConcurrentHashes(2)
	defer SetMaxConcurrentHashes(defaultMaxConcurrentHashes)

	const workers = 6
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = HashPassword("concurrent-password")
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestSetMaxConcurrentHashesClampsToMinimum(t *testing.T) {
	SetMaxConcurrentHashes(0)
	defer SetMaxConcurrentHashes(defaultMaxConcurrentHashes)

	hash, err := HashPassword("still-works")
	require.NoError(t, err)
	ok, _ := VerifyPassword(hash, "still-works")
	assert.True(t, ok)
}
