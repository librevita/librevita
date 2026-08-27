package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyEnvelopeScopesAndAuthentication(t *testing.T) {
	wrappingKey := bytes.Repeat([]byte{0x11}, SizeDEK)
	plaintext := bytes.Repeat([]byte{0x22}, SizeDEK)
	aad := []byte("urn:librevita:clinic:scope")

	envelope, err := wrapKey(wrappingKey, plaintext, keyScopeClinic, aad)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, envelope)

	got, err := unwrapKey(wrappingKey, envelope, keyScopeClinic, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	_, err = unwrapKey(wrappingKey, envelope, keyScopePatient, aad)
	assert.ErrorIs(t, err, ErrKeyScopeMismatch)

	tampered := append([]byte(nil), envelope...)
	tampered[len(tampered)-1] ^= 0xFF
	_, err = unwrapKey(wrappingKey, tampered, keyScopeClinic, aad)
	assert.Error(t, err)
}

func TestKeyEnvelopeRejectsMalformedValues(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, SizeDEK)
	_, err := wrapKey(key, []byte("short"), keyScopePatient, nil)
	assert.ErrorIs(t, err, ErrInvalidDEK)

	_, err = unwrapKey(key, []byte("short"), keyScopePatient, nil)
	assert.ErrorIs(t, err, ErrInvalidKeyEnvelope)
}
