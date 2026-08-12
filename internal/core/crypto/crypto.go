// Package crypto provides field-level encryption and blind indexing for
// sensitive patient data under a single master key.
//
// The master key is a base64-encoded 32-byte secret
// (LIBREVITA_MASTER_KEY). All derived keys come from HKDF-SHA256 with
// purpose-specific info strings, so the field-encryption key and the
// blind-index key never overlap: reusing one key for both would let an
// index leak information about the plaintext it protects.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// keySize is the byte length of the master key and of every derived
	// key.
	keySize = 32

	// fieldEncryptionInfo is the HKDF info string for the AEAD key.
	// Changing it rotates the effective encryption key; adding a suffix
	// migrates the scheme without touching the master key.
	fieldEncryptionInfo = "librevita:field-encryption:v1"

	// blindIndexInfo is the HKDF info string for the keyed-BLAKE2b key.
	blindIndexInfo = "librevita:blind-index:v1"

	// nonceSize is the XChaCha20-Poly1305 nonce length (24 bytes).
	nonceSize = chacha20poly1305.NonceSizeX
)

// MasterKey derives the purpose-specific keys of the encryption scheme.
type MasterKey struct {
	encKey   []byte
	blindKey []byte
}

// NewMasterKey parses the base64 32-byte master key. An empty or
// malformed value is an error: callers decide how to handle the
// development fallback, mirroring the PASETO key policy in
// internal/core/auth.
func NewMasterKey(encoded string) (*MasterKey, error) {
	if encoded == "" {
		return nil, errors.New("crypto: master key is empty")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: master key is not valid base64: %w", err)
	}
	if len(raw) != keySize {
		return nil, fmt.Errorf("crypto: master key must be 32 bytes, got %d", len(raw))
	}
	return derive(raw), nil
}

// derive expands the master key into the purpose-specific keys.
func derive(raw []byte) *MasterKey {
	return &MasterKey{
		encKey:   hkdfExpand(raw, fieldEncryptionInfo),
		blindKey: hkdfExpand(raw, blindIndexInfo),
	}
}

// hkdfExpand derives 32 bytes from ikm under the info string. The salt
// is nil: the master key has full entropy, so the info string alone
// separates the purposes.
func hkdfExpand(ikm []byte, info string) []byte {
	reader := hkdf.New(sha256.New, ikm, nil, []byte(info))
	out := make([]byte, keySize)
	if _, err := io.ReadFull(reader, out); err != nil {
		// HKDF-SHA256 cannot fail for a 32-byte output.
		panic(fmt.Sprintf("crypto: hkdf expand: %v", err))
	}
	return out
}

// Seal encrypts plaintext with XChaCha20-Poly1305 under a fresh random
// 24-byte nonce. aad binds the ciphertext to its context (the system
// URN of the identifier): tampering with the context fails Open, so a
// ciphertext moved to another system row cannot be decrypted. Returns
// the ciphertext and the nonce, which is not secret and is stored
// alongside.
func (k *MasterKey) Seal(aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	aead, err := chacha20poly1305.NewX(k.encKey)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: seal: %w", err)
	}
	nonce = make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

// Open authenticates and decrypts. Any tampering with ciphertext,
// nonce, or aad fails with an error.
func (k *MasterKey) Open(aad, ciphertext, nonce []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k.encKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// BlindIndex returns the hex keyed-BLAKE2b-256 digest of
// system || '\x00' || value under the blind-index key. It is
// deterministic: the same system and normalized value always produce
// the same 64-character index, which is what makes exact lookups
// possible without ever storing the plaintext. The null byte separates
// the fields, so a system of "ab" with value "c" never collides with
// system "a" and value "bc".
func (k *MasterKey) BlindIndex(system, value string) (string, error) {
	hasher, err := blake2b.New256(k.blindKey)
	if err != nil {
		return "", fmt.Errorf("crypto: blind index: %w", err)
	}
	hasher.Write([]byte(system))
	hasher.Write([]byte{0})
	hasher.Write([]byte(value))
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
