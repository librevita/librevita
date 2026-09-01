package crypto

import (
	"crypto/cipher"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/crypto/chacha20poly1305"
)

// Supported encryption ciphers and magic byte identifiers.
const (
	// CipherXChaCha20Poly1305 is the canonical identifier for XChaCha20-Poly1305 AEAD.
	CipherXChaCha20Poly1305 = "xchacha20-poly1305"

	// DefaultEncryptionCipher is the default encryption cipher.
	DefaultEncryptionCipher = CipherXChaCha20Poly1305

	// MagicByteXChaCha20Poly1305 indicates XChaCha20-Poly1305 AEAD with a 24-byte nonce.
	MagicByteXChaCha20Poly1305 byte = 0x01

	// DefaultEncryptionVersion is the default encryption Magic Byte (XChaCha20-Poly1305).
	DefaultEncryptionVersion = MagicByteXChaCha20Poly1305

	// CiphertextHeaderSize is the ciphertext prefix: magic, key scope, and key id.
	CiphertextHeaderSize = 3
)

// CipherSpec holds metadata and size parameters for a supported AEAD cipher.
type CipherSpec struct {
	Name      string
	NonceSize int
	TagSize   int
}

var supportedCiphers = map[byte]CipherSpec{
	MagicByteXChaCha20Poly1305: {
		Name:      CipherXChaCha20Poly1305,
		NonceSize: SizeNonce,   // 24
		TagSize:   SizeAuthTag, // 16
	},
}

// NewAEADCipher creates a standard AEAD cipher instance from a 32-byte key.
// By default, it initializes XChaCha20-Poly1305.
func NewAEADCipher(key []byte) (cipher.AEAD, error) {
	if len(key) < SizeDEK {
		return nil, ErrWeakKey
	}
	return chacha20poly1305.NewX(key)
}

// NewAEADCipherByVersion creates an AEAD cipher instance matching the specified magic byte version.
func NewAEADCipherByVersion(version byte, key []byte) (cipher.AEAD, error) {
	if len(key) < SizeDEK {
		return nil, ErrWeakKey
	}
	switch version {
	case MagicByteXChaCha20Poly1305:
		return chacha20poly1305.NewX(key)
	default:
		return nil, ErrUnsupportedVersion
	}
}

// MinCiphertextSizeForVersion returns the minimum ciphertext length for a specific magic byte version.
func MinCiphertextSizeForVersion(version byte) (int, bool) {
	spec, ok := supportedCiphers[version]
	if !ok {
		return 0, false
	}
	return CiphertextHeaderSize + spec.NonceSize + spec.TagSize, true
}

// IsCiphertext reports whether data begins with a recognized ciphertext Magic Byte,
// a valid key scope, and satisfies the minimum payload length for that cipher.
func IsCiphertext(data []byte) bool {
	if len(data) < CiphertextHeaderSize {
		return false
	}
	if !validDataKeyScope(data[1]) {
		return false
	}
	if !validKeyID(data[2]) {
		return false
	}
	minSize, ok := MinCiphertextSizeForVersion(data[0])
	if !ok {
		return false
	}
	return len(data) >= minSize
}

// IsCiphertextString reports whether a string represents a recognized ciphertext payload.
func IsCiphertextString(s string) bool {
	if len(s) < CiphertextHeaderSize {
		return false
	}
	if !validDataKeyScope(s[1]) {
		return false
	}
	if !validKeyID(s[2]) {
		return false
	}
	minSize, ok := MinCiphertextSizeForVersion(s[0])
	if !ok {
		return false
	}
	return len(s) >= minSize
}

func isValidVersion(v byte) bool {
	_, ok := supportedCiphers[v]
	return ok
}

func resolveCipherAndVersion(cipherName string, version byte) (byte, string, error) {
	if cipherName != "" && cipherName != DefaultEncryptionCipher {
		normalized := strings.ToLower(strings.TrimSpace(cipherName))
		for ver, spec := range supportedCiphers {
			if strings.EqualFold(spec.Name, normalized) || strings.EqualFold(strings.ReplaceAll(spec.Name, "-", ""), normalized) {
				return ver, spec.Name, nil
			}
		}
		return 0, "", errors.Wrapf(ErrUnsupportedVersion, "invalid encryption cipher %q", cipherName)
	}

	if spec, ok := supportedCiphers[version]; ok {
		return version, spec.Name, nil
	}
	return 0, "", ErrUnsupportedVersion
}
