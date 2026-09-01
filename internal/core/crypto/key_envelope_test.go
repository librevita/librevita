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

	envelope, err := wrapKey(wrappingKey, plaintext, KeyScopeClinic, DefaultKeyID, aad)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, envelope)
	assert.Equal(t, KeyScopeClinic, envelope[2])
	assert.Equal(t, byte('c'), envelope[2])

	got, err := unwrapKey(wrappingKey, envelope, KeyScopeClinic, DefaultKeyID, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	_, err = unwrapKey(wrappingKey, envelope, KeyScopePatient, DefaultKeyID, aad)
	assert.ErrorIs(t, err, ErrKeyScopeMismatch)

	_, err = unwrapKey(wrappingKey, envelope, KeyScopeClinic, 2, aad)
	assert.ErrorIs(t, err, ErrKeyIDMismatch)

	invalidScope := append([]byte(nil), envelope...)
	invalidScope[2] = 0
	_, err = unwrapKey(wrappingKey, invalidScope, KeyScopeClinic, DefaultKeyID, aad)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	tampered := append([]byte(nil), envelope...)
	tampered[len(tampered)-1] ^= 0xFF
	_, err = unwrapKey(wrappingKey, tampered, KeyScopeClinic, DefaultKeyID, aad)
	assert.Error(t, err)
}

func TestKeyEnvelopeRejectsMalformedValues(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, SizeDEK)
	_, err := wrapKey(key, []byte("short"), KeyScopePatient, DefaultKeyID, nil)
	assert.ErrorIs(t, err, ErrInvalidDEK)

	_, err = unwrapKey(key, []byte("short"), KeyScopePatient, DefaultKeyID, nil)
	assert.ErrorIs(t, err, ErrInvalidKeyEnvelope)

	dek := bytes.Repeat([]byte{0x44}, SizeDEK)
	_, err = wrapKey(key, dek, KeyScopeMaster, DefaultKeyID, nil)
	assert.ErrorIs(t, err, ErrInvalidKeyEnvelope)

	_, err = wrapKey(key, dek, 0, DefaultKeyID, nil)
	assert.ErrorIs(t, err, ErrInvalidKeyEnvelope)

	_, err = wrapKey(key, dek, KeyScopePatient, 0, nil)
	assert.ErrorIs(t, err, ErrInvalidKeyID)
}
