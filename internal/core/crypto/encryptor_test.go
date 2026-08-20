package crypto_test

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

const testEncryptorKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA="

func mustEncryptorKey(t *testing.T) []byte {
	t.Helper()
	k, err := base64.StdEncoding.DecodeString(testEncryptorKey)
	require.NoError(t, err)
	return k
}

func TestNewEncryptorFailFast(t *testing.T) {
	t.Run("rejects empty or weak key", func(t *testing.T) {
		_, err := crypto.NewEncryptor(nil)
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewEncryptor([]byte("short-key"))
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewEncryptorFromBase64("")
		assert.ErrorIs(t, err, crypto.ErrWeakKey)

		_, err = crypto.NewEncryptorFromBase64("invalid-base64!!")
		assert.Error(t, err)
	})

	t.Run("rejects unsupported or reserved cipher and version", func(t *testing.T) {
		k := mustEncryptorKey(t)
		_, err := crypto.NewEncryptor(k, crypto.WithEncryptionVersion(0xFF))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)

		// Version 0x02 is not supported
		_, err = crypto.NewEncryptor(k, crypto.WithEncryptionVersion(0x02))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)

		// Cipher name tests
		_, err = crypto.NewEncryptor(k, crypto.WithEncryptionCipher("aes-256-gcm"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)

		_, err = crypto.NewEncryptor(k, crypto.WithEncryptionCipher("chacha20poly1305"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)

		_, err = crypto.NewEncryptor(k, crypto.WithEncryptionCipher("chacha20-poly1305"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)

		_, err = crypto.NewEncryptor(k, crypto.WithEncryptionCipher("des-ede3-cbc"))
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)
	})
}

func TestEncryptorMagicByteXChaCha20Poly1305(t *testing.T) {
	k := mustEncryptorKey(t)

	enc, err := crypto.NewEncryptor(k, crypto.WithEncryptionCipher(crypto.CipherXChaCha20Poly1305))
	require.NoError(t, err)
	assert.Equal(t, crypto.MagicByteXChaCha20Poly1305, enc.Version())
	assert.Equal(t, crypto.CipherXChaCha20Poly1305, enc.Cipher())

	plaintext := []byte("confidential-medical-prescription-data")
	aad := []byte("urn:librevita:patient:018f-1234")

	ct, err := enc.Encrypt(plaintext, aad)
	require.NoError(t, err)

	// Must have Magic Byte 0x01 at [0]
	assert.Equal(t, crypto.MagicByteXChaCha20Poly1305, ct[0])
	// Length: 1 (header) + 24 (nonce) + len(plaintext) + 16 (tag)
	assert.Equal(t, 1+24+len(plaintext)+16, len(ct))

	// Decrypt
	decrypted, err := enc.Decrypt(ct, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptorReservedVersionDecryptFailFast(t *testing.T) {
	k := mustEncryptorKey(t)
	enc, err := crypto.NewEncryptor(k)
	require.NoError(t, err)

	// Fake ciphertext with unsupported magic byte 0x02
	fakeCT := make([]byte, 50)
	fakeCT[0] = 0x02

	_, err = enc.Decrypt(fakeCT, nil)
	assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)
}

func TestEncryptorTamperResistance(t *testing.T) {
	k := mustEncryptorKey(t)
	enc, err := crypto.NewEncryptor(k)
	require.NoError(t, err)

	plaintext := []byte("sensitive-health-record")
	aad := []byte("urn:librevita:domain:clinical")

	ct, err := enc.Encrypt(plaintext, aad)
	require.NoError(t, err)

	t.Run("tampered ciphertext payload fails", func(t *testing.T) {
		tampered := make([]byte, len(ct))
		copy(tampered, ct)
		tampered[len(tampered)-1] ^= 0xFF

		_, err := enc.Decrypt(tampered, aad)
		assert.ErrorIs(t, err, crypto.ErrDecryptionFailed)
	})

	t.Run("tampered nonce fails", func(t *testing.T) {
		tampered := make([]byte, len(ct))
		copy(tampered, ct)
		tampered[1] ^= 0xFF

		_, err := enc.Decrypt(tampered, aad)
		assert.ErrorIs(t, err, crypto.ErrDecryptionFailed)
	})

	t.Run("tampered magic byte fails", func(t *testing.T) {
		tampered := make([]byte, len(ct))
		copy(tampered, ct)
		tampered[0] = 0x99 // Invalid magic byte

		_, err := enc.Decrypt(tampered, aad)
		assert.ErrorIs(t, err, crypto.ErrUnsupportedVersion)
	})

	t.Run("mismatched AAD fails", func(t *testing.T) {
		_, err := enc.Decrypt(ct, []byte("wrong-aad"))
		assert.ErrorIs(t, err, crypto.ErrDecryptionFailed)
	})

	t.Run("truncated ciphertext fails", func(t *testing.T) {
		short := ct[:10]
		_, err := enc.Decrypt(short, aad)
		assert.ErrorIs(t, err, crypto.ErrCiphertextTooShort)
	})
}

type medicalPayloadTest struct {
	PatientID string   `json:"patient_id"`
	Diagnosis string   `json:"diagnosis"`
	MedCodes  []string `json:"med_codes"`
}

func TestEncryptorStructRoundtrip(t *testing.T) {
	k := mustEncryptorKey(t)
	enc, err := crypto.NewEncryptor(k)
	require.NoError(t, err)

	payload := medicalPayloadTest{
		PatientID: "urn:librevita:patient:100",
		Diagnosis: "Type 2 Diabetes Mellitus",
		MedCodes:  []string{"E11.9", "Z79.4"},
	}
	aad := []byte("urn:librevita:schema:medical_payload:v1")

	ct, err := enc.EncryptStruct(payload, aad)
	require.NoError(t, err)
	assert.NotEmpty(t, ct)

	var target medicalPayloadTest
	err = enc.DecryptInto(ct, aad, &target)
	require.NoError(t, err)
	assert.Equal(t, payload, target)

	// Tampered target struct decryption fails
	var badTarget medicalPayloadTest
	err = enc.DecryptInto(ct, []byte("bad-aad"), &badTarget)
	assert.Error(t, err)
}

func TestEncryptorKeyIsolation(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	_, _ = rand.Read(k1)
	_, _ = rand.Read(k2)

	enc1, err := crypto.NewEncryptor(k1)
	require.NoError(t, err)

	enc2, err := crypto.NewEncryptor(k2)
	require.NoError(t, err)

	plaintext := []byte("secret-diagnosis")
	ct, err := enc1.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Trying to decrypt with key 2 must fail
	_, err = enc2.Decrypt(ct, nil)
	assert.ErrorIs(t, err, crypto.ErrDecryptionFailed)
}

func BenchmarkEncryptor_XChaCha20Poly1305_Encrypt(b *testing.B) {
	k := []byte("01234567890123456789012345678901")
	enc, err := crypto.NewEncryptor(k, crypto.WithEncryptionVersion(crypto.MagicByteXChaCha20Poly1305))
	if err != nil {
		b.Fatal(err)
	}
	plaintext := []byte("confidential-medical-prescription-payload-benchmark")
	aad := []byte("urn:librevita:patient:100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enc.Encrypt(plaintext, aad)
	}
}

func BenchmarkEncryptor_XChaCha20Poly1305_Decrypt(b *testing.B) {
	k := []byte("01234567890123456789012345678901")
	enc, err := crypto.NewEncryptor(k, crypto.WithEncryptionVersion(crypto.MagicByteXChaCha20Poly1305))
	if err != nil {
		b.Fatal(err)
	}
	plaintext := []byte("confidential-medical-prescription-payload-benchmark")
	aad := []byte("urn:librevita:patient:100")
	ct, _ := enc.Encrypt(plaintext, aad)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enc.Decrypt(ct, aad)
	}
}

