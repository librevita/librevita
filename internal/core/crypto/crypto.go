// Package crypto provides field-level encryption, envelope encryption (KEK/DEK),
// key vault storage, and blind indexing for patient data under LibreVita.
//
// The master key is a base64-encoded 32-byte secret (LIBREVITA_MASTER_KEY).
// KEK (Key Encryption Key) and BlindIndexKey are derived via HKDF-BLAKE2b-256 with
// purpose-specific info strings:
//   - InfoKEK: "librevita:kek:v1" (used to wrap clinic DEKs)
//   - InfoBlindIndex: "librevita:blind-index:v1" (used for exact-match BLAKE2b blind indexes)
//
// Patient data is encrypted using XChaCha20-Poly1305 under a dedicated 32-byte
// random Data Encryption Key (DEK) per patient. Patient DEKs are wrapped by the
// clinic DEK and stored in a KeyVault. Deleting a patient's DEK from the vault
// executes instant Crypto-Shredding (GDPR/LGPD Right to be Forgotten).
package crypto

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// SizeDEK is the byte length of the KEK, DEK, and derived keys (32 bytes).
	SizeDEK = 32

	// InfoKEK is the HKDF info string for deriving the Key Encryption Key.
	InfoKEK = "librevita:kek:v1"

	// InfoBlindIndex is the HKDF info string for deriving the Blind Index Key.
	InfoBlindIndex = "librevita:blind-index:v1"

	// SizeNonce is the default AEAD nonce length (24 bytes).
	SizeNonce = 24

	// SizeAuthTag is the AEAD authentication tag length (16 bytes).
	SizeAuthTag = 16
)

// EngineOption configures an Engine instance.
type EngineOption func(*engineOptions)

type engineOptions struct {
	hashAlgorithm    string
	encryptionCipher string
}

// WithEngineHashAlgorithm sets the hash algorithm for blind indexing in the Engine.
func WithEngineHashAlgorithm(algo string) EngineOption {
	return func(o *engineOptions) {
		o.hashAlgorithm = algo
	}
}

// WithEngineEncryptionCipher sets the default encryption cipher for the Engine.
func WithEngineEncryptionCipher(cipher string) EngineOption {
	return func(o *engineOptions) {
		o.encryptionCipher = cipher
	}
}

// Engine orchestrates KEK, per-patient DEKs, Blind Indexing, and KeyVault storage.
type Engine struct {
	kek      []byte
	blindKey []byte
	vault    KeyVault
	metrics  *keyMetrics
	hasher   Hasher
	cipher   string
}

// MasterKey is an alias for Engine.
type MasterKey = Engine

// NewEngine initializes the crypto Engine from a base64 32-byte master key and KeyVault.
func NewEngine(masterKeyB64 string, vault KeyVault, opts ...EngineOption) (*Engine, error) {
	if masterKeyB64 == "" {
		return nil, errors.New("crypto: master key is empty")
	}
	if vault == nil {
		return nil, errors.New("crypto: key vault is required")
	}
	raw, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: master key is not valid base64: %w", err)
	}
	if len(raw) != SizeDEK {
		return nil, fmt.Errorf("crypto: master key must be 32 bytes, got %d", len(raw))
	}
	defer ZeroBytes(raw)
	return deriveEngine(raw, vault, opts...)
}

// NewMasterKey is a convenience alias for NewEngine.
func NewMasterKey(encoded string, vault KeyVault, opts ...EngineOption) (*Engine, error) {
	return NewEngine(encoded, vault, opts...)
}

func deriveEngine(raw []byte, vault KeyVault, opts ...EngineOption) (*Engine, error) {
	options := engineOptions{
		hashAlgorithm:    DefaultHashAlgorithm,
		encryptionCipher: DefaultEncryptionCipher,
	}
	for _, opt := range opts {
		opt(&options)
	}

	blindKey := hkdfExpand(raw, InfoBlindIndex)
	hasher, err := NewHasher(blindKey, WithHashAlgorithm(options.hashAlgorithm))
	if err != nil {
		ZeroBytes(blindKey)
		return nil, fmt.Errorf("crypto: engine hasher: %w", err)
	}

	return &Engine{
		kek:      hkdfExpand(raw, InfoKEK),
		blindKey: blindKey,
		vault:    vault,
		metrics:  &keyMetrics{},
		hasher:   hasher,
		cipher:   options.encryptionCipher,
	}, nil
}

// ZeroBytes securely zeroes the memory of a byte slice to prevent sensitive material from lingering in RAM.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SetupPatientDEK creates the patient-scoped DEK identified by a canonical
// PatientURN. The DEK is wrapped by the corresponding Clinic DEK and stored in
// the KeyVault. Returns the plaintext DEK for the current operation.
func (e *Engine) SetupPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := ParsePatientURN(patientURN)
	if !ok {
		return nil, fmt.Errorf("crypto: invalid patient urn %q", patientURN)
	}
	return e.SetupPatientDEKForClinic(ctx, clinicID, patientID)
}

// GetPatientDEK retrieves and unwraps a patient DEK identified by a canonical
// PatientURN from the KeyVault.
func (e *Engine) GetPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := ParsePatientURN(patientURN)
	if !ok {
		return nil, fmt.Errorf("crypto: invalid patient urn %q", patientURN)
	}
	return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
}

// DeletePatientDEK deletes a patient-scoped DEK identified by a canonical
// PatientURN, executing instant Crypto-Shredding.
func (e *Engine) DeletePatientDEK(ctx context.Context, patientURN string) error {
	if _, _, ok := ParsePatientURN(patientURN); !ok {
		return fmt.Errorf("crypto: invalid patient urn %q", patientURN)
	}
	err := e.vault.DeleteDEK(ctx, patientURN)
	if err == nil {
		forgetDEK(ctx, patientURN)
	}
	return err
}

// EnsurePatientDEK returns the existing patient DEK or creates one for a
// canonical PatientURN if it does not exist yet.
func (e *Engine) EnsurePatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := ParsePatientURN(patientURN)
	if !ok {
		return nil, fmt.Errorf("crypto: invalid patient urn %q", patientURN)
	}
	return e.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
}

// EncryptPatientData encrypts patient data using the patient's DEK from vault under XChaCha20-Poly1305.
func (e *Engine) EncryptPatientData(ctx context.Context, patientURN string, aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	return e.EncryptPayload(ctx, patientURN, aad, plaintext)
}

// DecryptPatientData decrypts patient data using the patient's DEK from the
// vault under XChaCha20-Poly1305. Returns ErrKeyDestroyed when the patient's
// DEK has been shredded.
func (e *Engine) DecryptPatientData(ctx context.Context, patientURN string, aad, ciphertext, nonce []byte) ([]byte, error) {
	return e.DecryptPayload(ctx, patientURN, aad, ciphertext, nonce)
}

// DecryptPatientDataWithDEK decrypts a payload with an already resolved
// patient DEK. It is intended for batch reads that have loaded each unique
// Patient DEK once.
func (e *Engine) DecryptPatientDataWithDEK(dek, aad, ciphertext, nonce []byte) ([]byte, error) {
	return decryptWithDEK(dek, aad, ciphertext, nonce)
}

// EncryptPayload encrypts any domain payload using the entity/patient DEK from vault under XChaCha20-Poly1305.
// Ensures that ephemeral keys in memory are wiped with ZeroBytes upon completion.
func (e *Engine) EncryptPayload(ctx context.Context, urn string, aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	dek, err := e.dekForURN(ctx, urn, true)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: get dek for encrypt: %w", err)
	}
	defer ZeroBytes(dek)

	return encryptWithDEK(dek, aad, plaintext)
}

// DecryptPayload decrypts any domain payload using the entity/patient DEK from vault under XChaCha20-Poly1305.
// Ensures that ephemeral keys in memory are wiped with ZeroBytes upon completion.
func (e *Engine) DecryptPayload(ctx context.Context, urn string, aad, ciphertext, nonce []byte) ([]byte, error) {
	dek, err := e.dekForURN(ctx, urn, false)
	if err != nil {
		return nil, fmt.Errorf("crypto: get dek for decrypt: %w", err)
	}
	defer ZeroBytes(dek)

	return decryptWithDEK(dek, aad, ciphertext, nonce)
}

func (e *Engine) dekForURN(ctx context.Context, urn string, ensure bool) ([]byte, error) {
	if clinicID, patientID, ok := ParsePatientURN(urn); ok {
		if ensure {
			return e.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
		}
		return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	}
	return nil, fmt.Errorf("crypto: payload urn must be patient-scoped: %q", urn)
}

// EncryptStruct serializes a Go struct to JSON and encrypts it using the entity's DEK.
// Transient plaintext JSON buffer is securely wiped with ZeroBytes.
func (e *Engine) EncryptStruct(ctx context.Context, urn string, aad []byte, source any) (ciphertext, nonce []byte, err error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: marshal struct: %w", err)
	}
	defer ZeroBytes(data)

	return e.EncryptPayload(ctx, urn, aad, data)
}

// DecryptInto decrypts a ciphertext payload and unmarshals the JSON into the target struct.
// Transient decrypted buffer is securely wiped with ZeroBytes.
func (e *Engine) DecryptInto(ctx context.Context, urn string, aad, ciphertext, nonce []byte, target any) error {
	plaintext, err := e.DecryptPayload(ctx, urn, aad, ciphertext, nonce)
	if err != nil {
		return err
	}
	defer ZeroBytes(plaintext)

	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("crypto: unmarshal decrypted payload: %w", err)
	}
	return nil
}

// EncryptField encrypts a string field under the entity's DEK with embedded nonce (24 bytes).
func (e *Engine) EncryptField(ctx context.Context, urn string, aad []byte, plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	data := []byte(plaintext)
	defer ZeroBytes(data)

	ct, nonce, err := e.EncryptPayload(ctx, urn, aad, data)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(nonce)+len(ct))
	copy(out[:len(nonce)], nonce)
	copy(out[len(nonce):], ct)
	return out, nil
}

// DecryptField decrypts a field with embedded nonce (24 bytes) under the entity's DEK.
func (e *Engine) DecryptField(ctx context.Context, urn string, aad []byte, encrypted []byte) (string, error) {
	if len(encrypted) < SizeNonce {
		return "", nil
	}
	nonce := encrypted[:SizeNonce]
	ct := encrypted[SizeNonce:]
	plaintext, err := e.DecryptPayload(ctx, urn, aad, ct, nonce)
	if err != nil {
		return "", err
	}
	defer ZeroBytes(plaintext)
	return string(plaintext), nil
}

// EncryptFieldPtr encrypts a nullable string pointer.
func (e *Engine) EncryptFieldPtr(ctx context.Context, urn string, aad []byte, plaintext *string) ([]byte, error) {
	if plaintext == nil || *plaintext == "" {
		return nil, nil
	}
	return e.EncryptField(ctx, urn, aad, *plaintext)
}

// DecryptFieldPtr decrypts nullable ciphertext bytes into a string pointer.
func (e *Engine) DecryptFieldPtr(ctx context.Context, urn string, aad []byte, encrypted []byte) (*string, error) {
	if len(encrypted) == 0 {
		return nil, nil
	}
	s, err := e.DecryptField(ctx, urn, aad, encrypted)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Seal encrypts plaintext using the global KEK directly for non-patient
// system secrets.
func (e *Engine) Seal(aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	aead, err := NewAEADCipher(e.kek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: seal: %w", err)
	}
	nonce, err = RandomBytes(SizeNonce)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

// Open authenticates and decrypts ciphertext encrypted with KEK directly.
func (e *Engine) Open(aad, ciphertext, nonce []byte) ([]byte, error) {
	aead, err := NewAEADCipher(e.kek)
	if err != nil {
		return nil, fmt.Errorf("crypto: open: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// BlindIndex returns the deterministic keyed digest of
// system || '\x00' || value under the global BlindIndexKey using the configured Hasher.
// It is deterministic across all patients to support exact matches (WHERE blind_index = ?).
func (e *Engine) BlindIndex(system, value string) (string, error) {
	if e.hasher != nil {
		return e.hasher.BlindIndex(system, value)
	}
	h, err := NewDigestWithKey(e.blindKey)
	if err != nil {
		return "", fmt.Errorf("crypto: blind index: %w", err)
	}
	h.Write([]byte(system))
	h.Write([]byte{0})
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func encryptWithDEK(dek, aad, plaintext []byte) ([]byte, []byte, error) {
	if len(dek) != SizeDEK {
		return nil, nil, ErrInvalidDEK
	}
	aead, err := NewAEADCipher(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: aead: %w", err)
	}
	nonce, err := RandomBytes(SizeNonce)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	return aead.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func decryptWithDEK(dek, aad, ciphertext, nonce []byte) ([]byte, error) {
	if len(dek) != SizeDEK {
		return nil, ErrInvalidDEK
	}
	aead, err := NewAEADCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: aead: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

func hkdfExpand(ikm []byte, info string) []byte {
	reader := hkdf.New(func() hash.Hash {
		h, err := NewDigest()
		if err != nil {
			panic(fmt.Sprintf("crypto: digest init: %v", err))
		}
		return h
	}, ikm, nil, []byte(info))
	out := make([]byte, SizeDEK)
	if _, err := io.ReadFull(reader, out); err != nil {
		panic(fmt.Sprintf("crypto: hkdf expand: %v", err))
	}
	return out
}
