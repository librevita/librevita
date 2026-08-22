package crypto_test

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

const testHasherKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA="

func mustHasherKey(t *testing.T) []byte {
	t.Helper()
	k, err := base64.StdEncoding.DecodeString(testHasherKey)
	require.NoError(t, err)
	return k
}

func TestNewHasherFailFast(t *testing.T) {
	t.Run("rejects empty or weak key", func(t *testing.T) {
		_, err := crypto.NewHasher(nil)
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewHasher([]byte("short-key"))
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewHasherFromBase64("")
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewHasherFromBase64("invalid-base64!!")
		assert.Error(t, err)
	})

	t.Run("rejects unsupported algorithm", func(t *testing.T) {
		k := mustHasherKey(t)
		_, err := crypto.NewHasher(k, crypto.WithHashAlgorithm("md5"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm)

		_, err = crypto.NewHasher(k, crypto.WithHashAlgorithm("sha1"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm)

		_, err = crypto.NewHasher(k, crypto.WithHashAlgorithm("bcrypt"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm)
	})
}

func TestHasherAllEnginesAllowlist(t *testing.T) {
	k := mustHasherKey(t)
	engines := []struct {
		name      string
		canonical string
	}{
		{"blake2s", "blake2s"},
		{"blake2b", "blake2b"},
	}

	for _, eng := range engines {
		t.Run(eng.name, func(t *testing.T) {
			hasher, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(eng.name))
			require.NoError(t, err)
			assert.Equal(t, eng.canonical, hasher.Algorithm())

			data := []byte("patient-session-token-jti-12345")
			hashed, err := hasher.Hash(data)
			require.NoError(t, err)

			// Format must be <algoritmo>$<hash_hex>
			parts := strings.Split(hashed, "$")
			require.Len(t, parts, 2)
			assert.Equal(t, eng.canonical, parts[0])
			assert.NotEmpty(t, parts[1])
			assert.True(t, isHex(parts[1]))

			// Verification must succeed
			ok, err := hasher.Verify(data, hashed)
			require.NoError(t, err)
			assert.True(t, ok)

			// Altered data must fail verification
			ok, err = hasher.Verify([]byte("tampered-data"), hashed)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

func TestHasherReservedEnginesFailFast(t *testing.T) {
	k := mustHasherKey(t)
	reserved := []string{
		"hmac-sha256",
		"sha256",
		"hmac-sha3-256",
		"sha3-256",
	}

	for _, algo := range reserved {
		t.Run(algo, func(t *testing.T) {
			_, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(algo))
			assert.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm)
		})
	}
}

func TestHasherCrossEngineVerificationAgility(t *testing.T) {
	k := mustHasherKey(t)

	hasherBlake2s, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2s))
	require.NoError(t, err)

	hasherBlake2b, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2b))
	require.NoError(t, err)

	data := []byte("confidential-document-identifier")

	// Hash with each engine
	hashBlake2s, err := hasherBlake2s.Hash(data)
	require.NoError(t, err)

	hashBlake2b, err := hasherBlake2b.Hash(data)
	require.NoError(t, err)

	// Single default hasher (blake2s) must successfully verify blake2b prefixed hashes!
	ok, err := hasherBlake2s.Verify(data, hashBlake2b)
	require.NoError(t, err)
	assert.True(t, ok)

	// Hasher blake2b must successfully verify blake2s prefixed hashes!
	ok, err = hasherBlake2b.Verify(data, hashBlake2s)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestHasherLegacyRawHexVerification(t *testing.T) {
	k := mustHasherKey(t)
	hasher, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2s))
	require.NoError(t, err)

	data := []byte("legacy-token")
	hashed, err := hasher.Hash(data)
	require.NoError(t, err)

	// Strip the prefix to simulate a legacy raw hex hash stored in DB
	rawHex := strings.Split(hashed, "$")[1]

	ok, err := hasher.Verify(data, rawHex)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = hasher.Verify([]byte("different-data"), rawHex)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHasherBlindIndexDomainSeparation(t *testing.T) {
	k := mustHasherKey(t)
	hasher, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2s))
	require.NoError(t, err)

	idx1, err := hasher.BlindIndex("urn:librevita:id:cpf", "12345678900")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(idx1, "blake2s$"))

	// Determinism
	idx2, err := hasher.BlindIndex("urn:librevita:id:cpf", "12345678900")
	require.NoError(t, err)
	assert.Equal(t, idx1, idx2)

	// Domain separation by system
	idx3, err := hasher.BlindIndex("urn:librevita:id:rg", "12345678900")
	require.NoError(t, err)
	assert.NotEqual(t, idx1, idx3)

	// Verification
	payload := []byte("urn:librevita:id:cpf" + "\x00" + "12345678900")
	ok, err := hasher.Verify(payload, idx1)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestHasherKeyIsolation(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	_, _ = rand.Read(k1)
	_, _ = rand.Read(k2)

	h1, err := crypto.NewHasher(k1)
	require.NoError(t, err)

	h2, err := crypto.NewHasher(k2)
	require.NoError(t, err)

	data := []byte("session-secret")
	hash1, err := h1.Hash(data)
	require.NoError(t, err)

	hash2, err := h2.Hash(data)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2)

	// h2 verifying hash1 must return false
	ok, err := h2.Verify(data, hash1)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHasherInvalidInputs(t *testing.T) {
	k := mustHasherKey(t)
	h, err := crypto.NewHasher(k)
	require.NoError(t, err)

	// Empty hash
	_, err = h.Verify([]byte("test"), "")
	assert.ErrorIs(t, err, crypto.ErrInvalidHashFormat)

	// Invalid prefix
	_, err = h.Verify([]byte("test"), "unknownalgo$abcdef")
	assert.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm)

	// Invalid hex
	_, err = h.Verify([]byte("test"), "blake2s$not-hex!!")
	assert.ErrorIs(t, err, crypto.ErrInvalidHashFormat)

	// Malformed prefix syntax
	_, err = h.Verify([]byte("test"), "$$$")
	assert.ErrorIs(t, err, crypto.ErrInvalidHashFormat)
}

func BenchmarkHasher_BLAKE2s(b *testing.B) {
	k := []byte("01234567890123456789012345678901")
	h, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2s))
	if err != nil {
		b.Fatal(err)
	}
	data := []byte("patient-session-token-benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Hash(data)
	}
}

func BenchmarkHasher_BLAKE2b(b *testing.B) {
	k := []byte("01234567890123456789012345678901")
	h, err := crypto.NewHasher(k, crypto.WithHashAlgorithm(crypto.AlgorithmBlake2b))
	if err != nil {
		b.Fatal(err)
	}
	data := []byte("patient-session-token-benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Hash(data)
	}
}

func BenchmarkHasher_Verify(b *testing.B) {
	k := []byte("01234567890123456789012345678901")
	h, err := crypto.NewHasher(k)
	if err != nil {
		b.Fatal(err)
	}
	data := []byte("patient-session-token-benchmark")
	hashStr, _ := h.Hash(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Verify(data, hashStr)
	}
}
