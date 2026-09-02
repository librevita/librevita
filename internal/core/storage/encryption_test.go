package storage

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

func TestEncryptedReaderRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, crypto.SizeDEK)
	plain := bytes.Repeat([]byte("clinical-data-"), encryptedFileChunkSize+123)
	aad := []byte("urn:librevita:clinic:patient")

	encrypted, err := NewEncryptedReader(bytes.NewReader(plain), key, aad)
	require.NoError(t, err)
	encoded, err := io.ReadAll(encrypted)
	require.NoError(t, err)
	require.NoError(t, encrypted.Close())
	assert.Equal(t, EncryptedSize(int64(len(plain))), int64(len(encoded)))
	assert.NotContains(t, encoded, plain)

	decrypted, err := NewDecryptedReader(bytes.NewReader(encoded), key, aad)
	require.NoError(t, err)
	got, err := io.ReadAll(decrypted)
	require.NoError(t, err)
	require.NoError(t, decrypted.Close())
	assert.Equal(t, plain, got)
}

func TestDecryptedReaderRejectsTamperedFrame(t *testing.T) {
	key := bytes.Repeat([]byte{0x24}, crypto.SizeDEK)
	encrypted, err := NewEncryptedReader(bytes.NewReader([]byte("secret")), key, []byte("aad"))
	require.NoError(t, err)
	encoded, err := io.ReadAll(encrypted)
	require.NoError(t, err)
	require.NoError(t, encrypted.Close())

	encoded[encryptedFileHeaderSize+encryptedFrameHeaderSize+2] ^= 0xFF
	decrypted, err := NewDecryptedReader(bytes.NewReader(encoded), key, []byte("aad"))
	require.NoError(t, err)
	_, err = io.ReadAll(decrypted)
	assert.ErrorIs(t, err, ErrInvalidEncryptedObject)
	require.NoError(t, decrypted.Close())
}

func TestDecryptedReaderRejectsWrongKeyScope(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, crypto.SizeDEK)
	encrypted, err := NewEncryptedReader(bytes.NewReader([]byte("secret")), key, []byte("aad"))
	require.NoError(t, err)
	encoded, err := io.ReadAll(encrypted)
	require.NoError(t, err)
	require.NoError(t, encrypted.Close())

	encoded[5] = crypto.KeyScopeClinic
	_, err = NewDecryptedReader(bytes.NewReader(encoded), key, []byte("aad"))
	assert.ErrorIs(t, err, crypto.ErrKeyScopeMismatch)
}

func TestDecryptedReaderRejectsWrongKeyID(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, crypto.SizeDEK)
	encrypted, err := NewEncryptedReader(bytes.NewReader([]byte("secret")), key, []byte("aad"))
	require.NoError(t, err)
	encoded, err := io.ReadAll(encrypted)
	require.NoError(t, err)
	require.NoError(t, encrypted.Close())

	encoded[6] = 2
	_, err = NewDecryptedReader(bytes.NewReader(encoded), key, []byte("aad"))
	assert.ErrorIs(t, err, crypto.ErrKeyIDMismatch)
}

func TestEncryptedReaderRejectsWeakKey(t *testing.T) {
	_, err := NewEncryptedReader(bytes.NewReader([]byte("data")), []byte("short"), nil)
	assert.ErrorIs(t, err, crypto.ErrInvalidDEK)
	_, err = NewDecryptedReader(bytes.NewReader(nil), []byte("short"), nil)
	assert.ErrorIs(t, err, crypto.ErrInvalidDEK)
}

func TestEncryptedSizeAndReaderEdgeCases(t *testing.T) {
	assert.Equal(t, int64(-1), EncryptedSize(-1))
	assert.Equal(t, int64(encryptedFileHeaderSize), EncryptedSize(0))
	assert.True(t, EncryptedSize(100) > 100)

	key := bytes.Repeat([]byte{0x77}, crypto.SizeDEK)
	aad := []byte("test-aad")

	// Nil source checks
	_, err := NewEncryptedReader(nil, key, aad)
	assert.Error(t, err)
	_, err = NewDecryptedReader(nil, key, aad)
	assert.Error(t, err)

	// Empty stream roundtrip
	enc, err := NewEncryptedReader(bytes.NewReader(nil), key, aad)
	require.NoError(t, err)
	encoded, err := io.ReadAll(enc)
	require.NoError(t, err)
	require.NoError(t, enc.Close())

	dec, err := NewDecryptedReader(bytes.NewReader(encoded), key, aad)
	require.NoError(t, err)
	plain, err := io.ReadAll(dec)
	require.NoError(t, err)
	assert.Empty(t, plain)
	require.NoError(t, dec.Close())

	// Magic byte tampering
	tamperedMagic := append([]byte(nil), encoded...)
	tamperedMagic[0] = 'X'
	_, err = NewDecryptedReader(bytes.NewReader(tamperedMagic), key, aad)
	assert.ErrorIs(t, err, ErrInvalidEncryptedObject)

	// Version tampering
	tamperedVer := append([]byte(nil), encoded...)
	tamperedVer[4] = 99
	_, err = NewDecryptedReader(bytes.NewReader(tamperedVer), key, aad)
	assert.ErrorIs(t, err, ErrInvalidEncryptedObject)
}
