package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHasherRejectsInvalidPurpose(t *testing.T) {
	k := make([]byte, SizeDEK)
	assert.Equal(t, byte('i'), KeyPurposeIndex)
	assert.Equal(t, byte('s'), KeyPurposeSession)

	_, err := newHasher(k, 0, KeyPurposeIndex, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	_, err = newHasher(k, KeyScopeMaster, 0, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyPurpose)

	_, err = newHasher(k, KeyScopeMaster, 0xFF, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyPurpose)

	_, err = newHasher(k, KeyScopeMaster, KeyPurposeIndex, 0)
	assert.ErrorIs(t, err, ErrInvalidKeyID)

	_, err = newHasher(k, KeyScopeClinic, KeyPurposeSession, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyPurpose)
}

func TestIndexAndSessionConstructorsStampPurpose(t *testing.T) {
	k := make([]byte, SizeDEK)
	for i := range k {
		k[i] = byte(i + 1)
	}

	ix, err := NewMasterIndexHasher(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopeMaster, ix.KeyScope())
	assert.Equal(t, KeyPurposeIndex, ix.KeyPurpose())
	assert.Equal(t, byte('i'), ix.KeyPurpose())
	assert.Equal(t, DefaultKeyID, ix.KeyID())

	clinic, err := NewHasherFromDEK(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopeClinic, clinic.KeyScope())
	assert.Equal(t, KeyPurposeIndex, clinic.KeyPurpose())
	assert.Equal(t, DefaultKeyID, clinic.KeyID())

	sess, err := NewSessionHasher(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopeMaster, sess.KeyScope())
	assert.Equal(t, KeyPurposeSession, sess.KeyPurpose())
	assert.Equal(t, DefaultKeyID, sess.KeyID())
	assert.Equal(t, byte('s'), sess.KeyPurpose())
}

func TestNewIndexHasherRejectsInvalidScopeAndKeyID(t *testing.T) {
	k := make([]byte, SizeDEK)
	_, err := newIndexHasher(k, 0, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	_, err = newIndexHasher(k, 0xFF, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	_, err = newIndexHasher(k, KeyScopeMaster, 0)
	assert.ErrorIs(t, err, ErrInvalidKeyID)
}

func TestIndexHasherKeyIDMismatch(t *testing.T) {
	k := make([]byte, SizeDEK)
	for i := range k {
		k[i] = byte(i + 1)
	}
	current, err := newIndexHasher(k, KeyScopeMaster, DefaultKeyID)
	require.NoError(t, err)
	other, err := newIndexHasher(k, KeyScopeMaster, 2)
	require.NoError(t, err)

	hashed, err := current.Hash([]byte("test"))
	require.NoError(t, err)
	_, err = other.Verify([]byte("test"), hashed)
	assert.ErrorIs(t, err, ErrKeyIDMismatch)
}
