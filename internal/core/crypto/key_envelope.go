package crypto

import (
	"errors"
	"fmt"
)

// KeyEnvelopeVersion identifies the envelope format used for wrapped DEKs.
const KeyEnvelopeVersion byte = 1

const (
	keyEnvelopeMagic      byte = 0xD1
	keyScopeClinic        byte = 1
	keyScopePatient       byte = 2
	keyEnvelopeHeaderSize      = 3
)

var (
	// ErrInvalidKeyEnvelope indicates a malformed or unexpected wrapped key.
	ErrInvalidKeyEnvelope = errors.New("crypto: invalid key envelope")
	// ErrKeyScopeMismatch indicates that a wrapped key belongs to another scope.
	ErrKeyScopeMismatch = errors.New("crypto: key envelope scope mismatch")
)

func wrapKey(wrappingKey, plaintext []byte, scope byte, aad []byte) ([]byte, error) {
	if len(wrappingKey) != SizeDEK {
		return nil, ErrWeakKey
	}
	if len(plaintext) != SizeDEK {
		return nil, ErrInvalidDEK
	}
	if !validKeyScope(scope) {
		return nil, ErrInvalidKeyEnvelope
	}

	aead, err := NewAEADCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: key envelope init: %w", err)
	}
	nonce, err := RandomBytes(SizeNonce)
	if err != nil {
		return nil, fmt.Errorf("crypto: key envelope nonce: %w", err)
	}

	header := []byte{keyEnvelopeMagic, KeyEnvelopeVersion, scope}
	out := make([]byte, 0, len(header)+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, header...)
	out = append(out, nonce...)
	out = aead.Seal(out, nonce, plaintext, appendEnvelopeAAD(header, aad))
	return out, nil
}

func unwrapKey(wrappingKey, envelope []byte, expectedScope byte, aad []byte) ([]byte, error) {
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
	if envelope[2] != expectedScope {
		return nil, ErrKeyScopeMismatch
	}

	header := envelope[:keyEnvelopeHeaderSize]
	nonceStart := keyEnvelopeHeaderSize
	nonceEnd := nonceStart + SizeNonce
	nonce := envelope[nonceStart:nonceEnd]
	ciphertext := envelope[nonceEnd:]

	aead, err := NewAEADCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: key envelope init: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, appendEnvelopeAAD(header, aad))
	if err != nil {
		return nil, fmt.Errorf("crypto: open key envelope: %w", err)
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

func validKeyScope(scope byte) bool {
	return scope == keyScopeClinic || scope == keyScopePatient
}
