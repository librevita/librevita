package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

func TestCipherConstructorsAndVersions(t *testing.T) {
	key, err := crypto.RandomBytes(crypto.SizeDEK)
	require.NoError(t, err)

	// Weak key rejection
	_, err = crypto.NewAEADCipher([]byte("short-key"))
	assert.ErrorIs(t, err, crypto.ErrWeakKey)

	_, err = crypto.NewAEADCipherByVersion(crypto.MagicByteXChaCha20Poly1305, []byte("short-key"))
	assert.ErrorIs(t, err, crypto.ErrWeakKey)

	// Valid cipher initialization
	aead, err := crypto.NewAEADCipher(key)
	require.NoError(t, err)
	assert.NotNil(t, aead)
	assert.Equal(t, crypto.SizeNonce, aead.NonceSize())
	assert.Equal(t, crypto.SizeAuthTag, aead.Overhead())

	// Version based initialization
	aeadVer, err := crypto.NewAEADCipherByVersion(crypto.MagicByteXChaCha20Poly1305, key)
	require.NoError(t, err)
	assert.NotNil(t, aeadVer)

	// Unsupported version
	_, err = crypto.NewAEADCipherByVersion(0xFF, key)
	assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)
}

func TestCipherMinSizeAndInspection(t *testing.T) {
	minSize, ok := crypto.MinCiphertextSizeForVersion(crypto.MagicByteXChaCha20Poly1305)
	assert.True(t, ok)
	assert.Equal(t, 1+crypto.SizeNonce+crypto.SizeAuthTag, minSize)

	_, ok = crypto.MinCiphertextSizeForVersion(0xFF)
	assert.False(t, ok)
}
