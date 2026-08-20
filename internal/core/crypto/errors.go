package crypto

import "errors"

var (
	// ErrWeakKey is returned when a provided cryptographic key is empty or below the required length (32 bytes).
	ErrWeakKey = errors.New("crypto: key is empty or too short (minimum 32 bytes required)")

	// ErrUnsupportedAlgorithm is returned when a hash algorithm is not in the allowlist.
	ErrUnsupportedAlgorithm = errors.New("crypto: unsupported hash algorithm")

	// ErrUnsupportedVersion is returned when an encryption Magic Byte version is not recognized.
	ErrUnsupportedVersion = errors.New("crypto: unsupported encryption magic byte version")

	// ErrCiphertextTooShort is returned when ciphertext is smaller than the required header + nonce + tag length.
	ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

	// ErrDecryptionFailed is returned when AEAD authentication or decryption fails.
	ErrDecryptionFailed = errors.New("crypto: decryption / authentication failed")

	// ErrInvalidHashFormat is returned when an encoded hash string is malformed.
	ErrInvalidHashFormat = errors.New("crypto: invalid hash format")
)
