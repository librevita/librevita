// Package crypto provides field-level encryption, envelope encryption (KEK/DEK),
// keystore storage, and blind indexing for patient data under LibreVita.
//
// The master key is a base64-encoded 32-byte secret (LIBREVITA_MASTER_KEY).
// KEK (Key Encryption Key) and BlindIndexKey are derived via HKDF-BLAKE2b-256 with
// purpose-specific info strings:
//   - InfoKEK: "librevita:kek:v1" (used to wrap clinic DEKs)
//   - InfoBlindIndex: "librevita:blind-index:v1" (used for exact-match BLAKE2b blind indexes)
//
// Patient data is encrypted using XChaCha20-Poly1305 under a dedicated 32-byte
// random Data Encryption Key (DEK) per patient. Patient DEKs are wrapped by the
// clinic DEK and stored in a KeyStore. Deleting a patient's DEK from the keystore
// executes instant Crypto-Shredding (GDPR/LGPD Right to be Forgotten).
package crypto

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"

	"github.com/cockroachdb/errors"
	"golang.org/x/crypto/hkdf"

	"librevita.org/pkg/urn"
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

// Engine orchestrates KEK, per-patient DEKs, Blind Indexing, and KeyStore storage.
type Engine struct {
	kek      []byte
	blindKey []byte
	keystore KeyStore
	metrics  *keyMetrics
	hasher   Hasher
	cipher   string
	kid      byte
}

// MasterKey is an alias for Engine.
type MasterKey = Engine

// NewEngine initializes the crypto Engine from a base64 32-byte master key and KeyStore.
func NewEngine(masterKeyB64 string, keystore KeyStore, opts ...EngineOption) (*Engine, error) {
	if masterKeyB64 == "" {
		return nil, errors.New("crypto: master key is empty")
	}
	if keystore == nil {
		return nil, errors.New("crypto: keystore is required")
	}
	raw, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: master key is not valid base64")
	}
	if len(raw) != SizeDEK {
		return nil, errors.Newf("crypto: master key must be 32 bytes, got %d", len(raw))
	}
	defer ZeroBytes(raw)
	return deriveEngine(raw, keystore, opts...)
}

// NewMasterKey is a convenience alias for NewEngine.
func NewMasterKey(encoded string, keystore KeyStore, opts ...EngineOption) (*Engine, error) {
	return NewEngine(encoded, keystore, opts...)
}

func deriveEngine(raw []byte, keystore KeyStore, opts ...EngineOption) (*Engine, error) {
	options := engineOptions{
		hashAlgorithm:    DefaultHashAlgorithm,
		encryptionCipher: DefaultEncryptionCipher,
	}
	for _, opt := range opts {
		opt(&options)
	}

	hasher, err := newIndexHasher(raw, KeyScopeMaster, DefaultKeyID, WithHashAlgorithm(options.hashAlgorithm))
	if err != nil {
		return nil, errors.Wrap(err, "crypto: engine hasher")
	}
	blindKey := hkdfExpand(raw, InfoBlindIndex)

	return &Engine{
		kek:      hkdfExpand(raw, InfoKEK),
		blindKey: blindKey,
		keystore: keystore,
		metrics:  &keyMetrics{},
		hasher:   hasher,
		cipher:   options.encryptionCipher,
		kid:      DefaultKeyID,
	}, nil
}

// ZeroBytes securely zeroes the memory of a byte slice to prevent sensitive material from lingering in RAM.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SetupPatientDEK creates the patient-scoped DEK identified by a canonical
// patient URN. The DEK is wrapped by the corresponding Clinic DEK and stored in
// the KeyStore. Returns the plaintext DEK for the current operation.
func (e *Engine) SetupPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := urn.ParsePatient(patientURN)
	if !ok {
		return nil, errors.Newf("crypto: invalid patient urn %q", patientURN)
	}
	return e.SetupPatientDEKForClinic(ctx, clinicID, patientID)
}

// GetPatientDEK retrieves and unwraps a patient DEK identified by a canonical
// patient URN from the KeyStore.
func (e *Engine) GetPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := urn.ParsePatient(patientURN)
	if !ok {
		return nil, errors.Newf("crypto: invalid patient urn %q", patientURN)
	}
	return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
}

// DeletePatientDEK deletes a patient-scoped DEK identified by a canonical
// patient URN, executing instant Crypto-Shredding.
func (e *Engine) DeletePatientDEK(ctx context.Context, patientURN string) error {
	if _, _, ok := urn.ParsePatient(patientURN); !ok {
		return errors.Newf("crypto: invalid patient urn %q", patientURN)
	}
	err := e.keystore.DeleteDEK(ctx, patientURN)
	if err == nil {
		forgetDEK(ctx, patientURN)
	}
	return err
}

// EnsurePatientDEK returns the existing patient DEK or creates one for a
// canonical patient URN if it does not exist yet.
func (e *Engine) EnsurePatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	clinicID, patientID, ok := urn.ParsePatient(patientURN)
	if !ok {
		return nil, errors.Newf("crypto: invalid patient urn %q", patientURN)
	}
	return e.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
}

// EncryptPatientData encrypts patient data using the patient's DEK from the keystore under XChaCha20-Poly1305.
func (e *Engine) EncryptPatientData(ctx context.Context, patientURN string, aad, plaintext []byte) ([]byte, error) {
	return e.EncryptPayload(ctx, patientURN, aad, plaintext)
}

// DecryptPatientData decrypts patient data using the patient's DEK from the
// keystore under XChaCha20-Poly1305. Returns ErrKeyDestroyed when the patient's
// DEK has been shredded.
func (e *Engine) DecryptPatientData(ctx context.Context, patientURN string, aad, ciphertext []byte) ([]byte, error) {
	return e.DecryptPayload(ctx, patientURN, aad, ciphertext)
}

// DecryptPatientDataWithDEK decrypts a payload with an already resolved
// patient DEK. It is intended for batch reads that have loaded each unique
// Patient DEK once.
func (e *Engine) DecryptPatientDataWithDEK(dek, aad, ciphertext []byte) ([]byte, error) {
	enc, err := newEncryptor(dek, KeyScopePatient, e.kid, WithEncryptionCipher(e.cipher))
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(enc.key)
	return enc.Decrypt(ciphertext, aad)
}

// EncryptPayload encrypts any domain payload using the entity DEK from the keystore.
// Ensures that ephemeral keys in memory are wiped with ZeroBytes upon completion.
func (e *Engine) EncryptPayload(ctx context.Context, key string, aad, plaintext []byte) ([]byte, error) {
	dek, err := e.dekForURN(ctx, key, true)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: get dek for encrypt")
	}
	defer ZeroBytes(dek)

	enc, err := e.encryptorForURN(dek, key)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(enc.key)
	return enc.Encrypt(plaintext, aad)
}

// DecryptPayload decrypts any domain payload using the entity DEK from the keystore.
// Ensures that ephemeral keys in memory are wiped with ZeroBytes upon completion.
func (e *Engine) DecryptPayload(ctx context.Context, key string, aad, ciphertext []byte) ([]byte, error) {
	dek, err := e.dekForURN(ctx, key, false)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: get dek for decrypt")
	}
	defer ZeroBytes(dek)

	enc, err := e.encryptorForURN(dek, key)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(enc.key)
	return enc.Decrypt(ciphertext, aad)
}

func (e *Engine) encryptorForURN(dek []byte, key string) (*AEADEncryptor, error) {
	scope, err := keyScopeForURN(key)
	if err != nil {
		return nil, err
	}
	return newEncryptor(dek, scope, e.kid, WithEncryptionCipher(e.cipher))
}

func keyScopeForURN(key string) (byte, error) {
	if _, _, ok := urn.ParsePatient(key); ok {
		return KeyScopePatient, nil
	}
	if _, ok := urn.ParseClinic(key); ok {
		return KeyScopeClinic, nil
	}
	return 0, errors.Newf("crypto: payload urn must be clinic- or patient-scoped: %q", key)
}

func (e *Engine) dekForURN(ctx context.Context, key string, ensure bool) ([]byte, error) {
	if clinicID, patientID, ok := urn.ParsePatient(key); ok {
		if ensure {
			return e.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
		}
		return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	}
	if clinicID, ok := urn.ParseClinic(key); ok {
		if ensure {
			return e.EnsureClinicDEK(ctx, clinicID)
		}
		return e.GetClinicDEK(ctx, clinicID)
	}
	return nil, errors.Newf("crypto: payload urn must be clinic- or patient-scoped: %q", key)
}

// EncryptStruct serializes a Go struct to JSON and encrypts it using the entity's DEK.
// Transient plaintext JSON buffer is securely wiped with ZeroBytes.
func (e *Engine) EncryptStruct(ctx context.Context, urn string, aad []byte, source any) ([]byte, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, errors.Wrap(err, "crypto: marshal struct")
	}
	defer ZeroBytes(data)

	return e.EncryptPayload(ctx, urn, aad, data)
}

// DecryptInto decrypts a ciphertext payload and unmarshals the JSON into the target struct.
// Transient decrypted buffer is securely wiped with ZeroBytes.
func (e *Engine) DecryptInto(ctx context.Context, urn string, aad, ciphertext []byte, target any) error {
	plaintext, err := e.DecryptPayload(ctx, urn, aad, ciphertext)
	if err != nil {
		return err
	}
	defer ZeroBytes(plaintext)

	if err := json.Unmarshal(plaintext, target); err != nil {
		return errors.Wrap(err, "crypto: unmarshal decrypted payload")
	}
	return nil
}

// EncryptField encrypts a string field under the entity's DEK as a self-describing envelope.
func (e *Engine) EncryptField(ctx context.Context, urn string, aad []byte, plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	data := []byte(plaintext)
	defer ZeroBytes(data)
	return e.EncryptPayload(ctx, urn, aad, data)
}

// DecryptField decrypts a self-describing field envelope under the entity's DEK.
func (e *Engine) DecryptField(ctx context.Context, urn string, aad []byte, encrypted []byte) (string, error) {
	if len(encrypted) == 0 {
		return "", nil
	}
	plaintext, err := e.DecryptPayload(ctx, urn, aad, encrypted)
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
func (e *Engine) Seal(aad, plaintext []byte) ([]byte, error) {
	enc, err := newEncryptor(e.kek, KeyScopeMaster, e.kid, WithEncryptionCipher(e.cipher))
	if err != nil {
		return nil, errors.Wrap(err, "crypto: seal")
	}
	defer ZeroBytes(enc.key)
	return enc.Encrypt(plaintext, aad)
}

// Open authenticates and decrypts ciphertext encrypted with KEK directly.
func (e *Engine) Open(aad, ciphertext []byte) ([]byte, error) {
	enc, err := newEncryptor(e.kek, KeyScopeMaster, e.kid, WithEncryptionCipher(e.cipher))
	if err != nil {
		return nil, errors.Wrap(err, "crypto: open")
	}
	defer ZeroBytes(enc.key)
	return enc.Decrypt(ciphertext, aad)
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
		return "", errors.Wrap(err, "crypto: blind index")
	}
	h.Write([]byte(system))
	h.Write([]byte{0})
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil)), nil
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
