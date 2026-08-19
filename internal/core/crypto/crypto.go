// Package crypto provides field-level encryption, envelope encryption (KEK/DEK),
// key vault storage, and blind indexing for patient data under LibreVita.
//
// The master key is a base64-encoded 32-byte secret (LIBREVITA_MASTER_KEY).
// KEK (Key Encryption Key) and BlindIndexKey are derived via HKDF-BLAKE2b-256 with
// purpose-specific info strings:
//   - InfoKEK: "librevita:kek:v1" (used ONLY to encrypt/decrypt patient DEKs)
//   - InfoBlindIndex: "librevita:blind-index:v1" (used for exact-match BLAKE2b blind indexes)
//
// Patient data is encrypted using XChaCha20-Poly1305 under a dedicated 32-byte
// random Data Encryption Key (DEK) per patient. The DEK is encrypted with the KEK
// and stored in a KeyVault (bbolt). Deleting a patient's DEK from the vault
// executes instant Crypto-Shredding (GDPR/LGPD Right to be Forgotten).
package crypto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// SizeDEK is the byte length of the KEK, DEK, and derived keys (32 bytes).
	SizeDEK = 32

	// InfoKEK is the HKDF info string for deriving the Key Encryption Key.
	InfoKEK = "librevita:kek:v1"

	// InfoBlindIndex is the HKDF info string for deriving the Blind Index Key.
	InfoBlindIndex = "librevita:blind-index:v1"

	// SizeNonce is the XChaCha20-Poly1305 nonce length (24 bytes).
	SizeNonce = chacha20poly1305.NonceSizeX
)

// Engine orchestrates KEK, per-patient DEKs, Blind Indexing, and KeyVault storage.
type Engine struct {
	kek      []byte
	blindKey []byte
	vault    KeyVault
}

// MasterKey is an alias for Engine for backwards compatibility.
type MasterKey = Engine

// NewEngine initializes the crypto Engine from a base64 32-byte master key and KeyVault.
func NewEngine(masterKeyB64 string, vault KeyVault) (*Engine, error) {
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
	return deriveEngine(raw, vault), nil
}

// NewMasterKey is a convenience alias for NewEngine.
func NewMasterKey(encoded string, vault KeyVault) (*Engine, error) {
	return NewEngine(encoded, vault)
}

func deriveEngine(raw []byte, vault KeyVault) *Engine {
	return &Engine{
		kek:      hkdfExpand(raw, InfoKEK),
		blindKey: hkdfExpand(raw, InfoBlindIndex),
		vault:    vault,
	}
}

// SetupPatientDEK generates a fresh random 32-byte DEK for patientURN, encrypts it
// with the KEK, and stores it in the KeyVault. Returns the plaintext DEK.
func (e *Engine) SetupPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	dek := make([]byte, SizeDEK)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("crypto: generate dek: %w", err)
	}

	encDEK, err := e.encryptWithKEK(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: encrypt dek: %w", err)
	}

	if err := e.vault.PutDEK(ctx, patientURN, encDEK); err != nil {
		return nil, fmt.Errorf("crypto: save dek to vault: %w", err)
	}

	return dek, nil
}

// GetPatientDEK retrieves and decrypts the DEK for patientURN from the KeyVault.
// If not found, returns ErrKeyNotFound.
func (e *Engine) GetPatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	encDEK, err := e.vault.GetDEK(ctx, patientURN)
	if err != nil {
		return nil, err
	}
	return e.decryptWithKEK(encDEK)
}

// DeletePatientDEK deletes the patient's DEK from the vault, executing instant Crypto-Shredding.
func (e *Engine) DeletePatientDEK(ctx context.Context, patientURN string) error {
	return e.vault.DeleteDEK(ctx, patientURN)
}

// EnsurePatientDEK returns the existing patient DEK or creates a new one if it does not exist yet.
func (e *Engine) EnsurePatientDEK(ctx context.Context, patientURN string) ([]byte, error) {
	dek, err := e.GetPatientDEK(ctx, patientURN)
	if errors.Is(err, ErrKeyNotFound) {
		return e.SetupPatientDEK(ctx, patientURN)
	}
	if err != nil {
		return nil, err
	}
	return dek, nil
}

// EncryptPatientData encrypts patient data using the patient's DEK from vault under XChaCha20-Poly1305.
func (e *Engine) EncryptPatientData(ctx context.Context, patientURN string, aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	dek, err := e.EnsurePatientDEK(ctx, patientURN)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: get dek for encrypt: %w", err)
	}

	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: aead: %w", err)
	}

	nonce = make([]byte, SizeNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: nonce: %w", err)
	}

	ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

// DecryptPatientData decrypts patient data using the patient's DEK from vault under XChaCha20-Poly1305.
// Returns ErrKeyNotFound if the patient's DEK has been deleted (Crypto-Shredded).
func (e *Engine) DecryptPatientData(ctx context.Context, patientURN string, aad, ciphertext, nonce []byte) ([]byte, error) {
	dek, err := e.GetPatientDEK(ctx, patientURN)
	if err != nil {
		return nil, fmt.Errorf("crypto: get dek for decrypt: %w", err)
	}

	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: aead: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}

	return plaintext, nil
}

// Seal encrypts plaintext using the global KEK directly.
// (Used for non-patient system secrets or backwards compatibility).
func (e *Engine) Seal(aad, plaintext []byte) (ciphertext, nonce []byte, err error) {
	aead, err := chacha20poly1305.NewX(e.kek)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto: seal: %w", err)
	}
	nonce = make([]byte, SizeNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: nonce: %w", err)
	}
	ciphertext = aead.Seal(nil, nonce, plaintext, aad)
	return ciphertext, nonce, nil
}

// Open authenticates and decrypts ciphertext encrypted with KEK directly.
func (e *Engine) Open(aad, ciphertext, nonce []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(e.kek)
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
// system || '\x00' || value under the global BlindIndexKey.
// It is deterministic across all patients to support exact matches (WHERE blind_index = ?).
func (e *Engine) BlindIndex(system, value string) (string, error) {
	hasher, err := blake2b.New256(e.blindKey)
	if err != nil {
		return "", fmt.Errorf("crypto: blind index: %w", err)
	}
	hasher.Write([]byte(system))
	hasher.Write([]byte{0})
	hasher.Write([]byte(value))
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (e *Engine) encryptWithKEK(plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(e.kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, SizeNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, SizeNonce+len(ciphertext))
	copy(out[:SizeNonce], nonce)
	copy(out[SizeNonce:], ciphertext)
	return out, nil
}

func (e *Engine) decryptWithKEK(data []byte) ([]byte, error) {
	if len(data) < SizeNonce {
		return nil, errors.New("crypto: ciphertext too short for KEK decryption")
	}
	nonce := data[:SizeNonce]
	ciphertext := data[SizeNonce:]

	aead, err := chacha20poly1305.NewX(e.kek)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt dek with kek: %w", err)
	}
	return plaintext, nil
}

func newBLAKE2b256() hash.Hash {
	h, err := blake2b.New256(nil)
	if err != nil {
		panic(fmt.Sprintf("crypto: blake2b init: %v", err))
	}
	return h
}

func hkdfExpand(ikm []byte, info string) []byte {
	reader := hkdf.New(newBLAKE2b256, ikm, nil, []byte(info))
	out := make([]byte, SizeDEK)
	if _, err := io.ReadFull(reader, out); err != nil {
		panic(fmt.Sprintf("crypto: hkdf expand: %v", err))
	}
	return out
}
