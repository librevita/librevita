package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncryptorRejectsInvalidScopeAndKeyID(t *testing.T) {
	k := make([]byte, SizeDEK)
	_, err := newEncryptor(k, 0, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	_, err = newEncryptor(k, 0xFF, DefaultKeyID)
	assert.ErrorIs(t, err, ErrInvalidKeyScope)

	_, err = newEncryptor(k, KeyScopePatient, 0)
	assert.ErrorIs(t, err, ErrInvalidKeyID)
}

func TestNamedEncryptorsStampScopeAndDefaultKeyID(t *testing.T) {
	k := make([]byte, SizeDEK)
	for i := range k {
		k[i] = byte(i + 1)
	}

	master, err := NewMasterEncryptor(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopeMaster, master.KeyScope())
	assert.Equal(t, DefaultKeyID, master.KeyID())

	clinic, err := NewClinicEncryptor(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopeClinic, clinic.KeyScope())
	assert.Equal(t, DefaultKeyID, clinic.KeyID())

	patient, err := NewPatientEncryptor(k)
	require.NoError(t, err)
	assert.Equal(t, KeyScopePatient, patient.KeyScope())
	assert.Equal(t, DefaultKeyID, patient.KeyID())
}

func TestEncryptorConstructKeyIDMismatch(t *testing.T) {
	k := make([]byte, SizeDEK)
	for i := range k {
		k[i] = byte(i + 1)
	}
	current, err := newEncryptor(k, KeyScopePatient, DefaultKeyID)
	require.NoError(t, err)
	other, err := newEncryptor(k, KeyScopePatient, 2)
	require.NoError(t, err)

	ct, err := current.Encrypt([]byte("phi"), nil)
	require.NoError(t, err)
	_, err = other.Decrypt(ct, nil)
	assert.ErrorIs(t, err, ErrKeyIDMismatch)
}
