package crypto

import (
	"github.com/cockroachdb/errors"
)

// KeyEnvelopeVersion identifies the envelope format used for wrapped DEKs.
const KeyEnvelopeVersion byte = 1

// DefaultKeyID is the current key generation. Byte 0 is invalid so a
// zero-value encryptor or hasher cannot stamp a kid. Rotation keeps
// DefaultKeyID as the sealing generation and retains retired ids in a
// keyring for Open; this constant is not a format version.
const DefaultKeyID byte = 1

// Key hierarchy tiers stamped into wrapped DEKs, data ciphertext, and keyed hashes.
// The values are the ASCII letters also used in hash tokens. Byte 0 is invalid
// so a zero-value encryptor or hasher cannot stamp a scope.
const (
	// KeyScopeMaster is the process KEK (ciphertext) and master-derived
	// hash keys (HKDF from LIBREVITA_MASTER_KEY, not the raw master bytes).
	// Hash purpose (index vs session) is a separate stamp on Hasher output.
	KeyScopeMaster byte = 'm'
	// KeyScopeClinic is a clinic DEK (and the hasher derived from it).
	KeyScopeClinic byte = 'c'
	// KeyScopePatient is a patient DEK.
	KeyScopePatient byte = 'p'
)

const (
	keyEnvelopeMagic      byte = 0xD1
	keyEnvelopeHeaderSize      = 4
)

var (
	// ErrInvalidKeyEnvelope indicates a malformed or unexpected wrapped key.
	ErrInvalidKeyEnvelope = errors.New("crypto: invalid key envelope")
	// ErrInvalidKeyScope indicates a key scope byte or hash token is not recognized.
	ErrInvalidKeyScope = errors.New("crypto: invalid key scope")
	// ErrKeyScopeMismatch indicates ciphertext, a hash, or a wrapped key belongs to another scope.
	ErrKeyScopeMismatch = errors.New("crypto: key envelope scope mismatch")
	// ErrInvalidKeyPurpose indicates a hash purpose byte or token is not recognized.
	ErrInvalidKeyPurpose = errors.New("crypto: invalid key purpose")
	// ErrKeyPurposeMismatch indicates a hash was produced for another key purpose.
	ErrKeyPurposeMismatch = errors.New("crypto: key purpose mismatch")
	// ErrInvalidKeyID indicates a key id byte or hash token is not recognized.
	ErrInvalidKeyID = errors.New("crypto: invalid key id")
	// ErrKeyIDMismatch indicates ciphertext, a hash, or a wrapped key belongs to another key generation.
	ErrKeyIDMismatch = errors.New("crypto: key id mismatch")
)

func wrapKey(wrappingKey, plaintext []byte, scope, kid byte, aad []byte) ([]byte, error) {
	if len(wrappingKey) != SizeDEK {
		return nil, ErrWeakKey
	}
	if len(plaintext) != SizeDEK {
		return nil, ErrInvalidDEK
	}
	if !validWrappedKeyScope(scope) {
		return nil, ErrInvalidKeyEnvelope
	}
	if !validKeyID(kid) {
		return nil, ErrInvalidKeyID
	}

	aead, err := NewAEADCipher(wrappingKey)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: key envelope init")
	}
	nonce, err := RandomBytes(SizeNonce)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: key envelope nonce")
	}

	header := []byte{keyEnvelopeMagic, KeyEnvelopeVersion, scope, kid}
	out := make([]byte, 0, len(header)+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, header...)
	out = append(out, nonce...)
	out = aead.Seal(out, nonce, plaintext, appendEnvelopeAAD(header, aad))
	return out, nil
}

func unwrapKey(wrappingKey, envelope []byte, expectedScope, expectedKid byte, aad []byte) ([]byte, error) {
	if len(wrappingKey) != SizeDEK {
		return nil, ErrWeakKey
	}
	minSize := keyEnvelopeHeaderSize + SizeNonce + SizeDEK + SizeAuthTag
	if len(envelope) < minSize {
		return nil, ErrInvalidKeyEnvelope
	}
	if envelope[0] != keyEnvelopeMagic || envelope[1] != KeyEnvelopeVersion {
		return nil, ErrInvalidKeyEnvelope
	}
	if !validWrappedKeyScope(envelope[2]) {
		return nil, ErrInvalidKeyScope
	}
	if envelope[2] != expectedScope {
		return nil, ErrKeyScopeMismatch
	}
	if !validKeyID(envelope[3]) {
		return nil, ErrInvalidKeyID
	}
	if envelope[3] != expectedKid {
		return nil, ErrKeyIDMismatch
	}

	header := envelope[:keyEnvelopeHeaderSize]
	nonceStart := keyEnvelopeHeaderSize
	nonceEnd := nonceStart + SizeNonce
	nonce := envelope[nonceStart:nonceEnd]
	ciphertext := envelope[nonceEnd:]

	aead, err := NewAEADCipher(wrappingKey)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: key envelope init")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, appendEnvelopeAAD(header, aad))
	if err != nil {
		return nil, errors.Wrap(err, "crypto: open key envelope")
	}
	if len(plaintext) != SizeDEK {
		ZeroBytes(plaintext)
		return nil, ErrInvalidDEK
	}
	return plaintext, nil
}

func appendEnvelopeAAD(header, aad []byte) []byte {
	out := make([]byte, 0, len(header)+len(aad))
	out = append(out, header...)
	out = append(out, aad...)
	return out
}

func validWrappedKeyScope(scope byte) bool {
	return scope == KeyScopeClinic || scope == KeyScopePatient
}

func validDataKeyScope(scope byte) bool {
	return scope == KeyScopeMaster || scope == KeyScopeClinic || scope == KeyScopePatient
}

func validKeyID(id byte) bool {
	return id != 0
}
