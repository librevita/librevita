package crypto

import (
	"bytes"
	"context"

	"github.com/cockroachdb/errors"
)

var (
	// ErrKeyNotFound indicates the requested DEK does not exist in the vault.
	ErrKeyNotFound = errors.New("crypto: dek not found in vault")

	// ErrKeyDestroyed indicates that the key was permanently shredded and must
	// never be recreated for the same URN.
	ErrKeyDestroyed = errors.New("crypto: dek permanently destroyed")

	// ErrVaultClosed indicates operations were attempted on a closed KeyVault.
	ErrVaultClosed = errors.New("crypto: key vault is closed")

	// ErrInvalidDEK indicates a malformed or invalid DEK payload.
	ErrInvalidDEK = errors.New("crypto: invalid DEK size or format")
)

var destroyedDEKMarker = []byte{0x00, 'L', 'V', '-', 'D', 'E', 'S', 'T', 'R', 'O', 'Y', 'E', 'D'}

// DestroyedDEKMarker returns a copy of the non-secret tombstone stored by
// vault adapters after a DEK is shredded.
func DestroyedDEKMarker() []byte {
	return append([]byte(nil), destroyedDEKMarker...)
}

// IsDestroyedDEK reports whether a vault value is a terminal shredding
// tombstone rather than an encrypted DEK.
func IsDestroyedDEK(value []byte) bool {
	return bytes.Equal(value, destroyedDEKMarker)
}

// KeyVault defines the Hexagonal Port interface for storing and retrieving
// encrypted patient Data Encryption Keys (DEKs).
//
// Implementations can range from embedded Key-Value databases (e.g. bbolt)
// for single-node deployments to enterprise remote KMS/Vault solutions
// (HashiCorp Vault, AWS KMS, GCP Key Management) for multi-node clusters.
type KeyVault interface {
	// PutDEK stores encrypted DEK bytes associated with a key URN.
	PutDEK(ctx context.Context, urn string, encryptedDEK []byte) error

	// GetDEK retrieves encrypted DEK bytes for a key URN.
	// Returns ErrKeyNotFound if no key exists for the given URN.
	GetDEK(ctx context.Context, urn string) ([]byte, error)

	// DeleteDEK permanently deletes the DEK from storage,
	// executing instant Crypto-Shredding for GDPR/LGPD Right to be Forgotten.
	DeleteDEK(ctx context.Context, urn string) error

	// Close cleanly releases any vault storage resources or connections.
	Close() error
}

// DEKResult is the result for one item returned by a batch vault lookup.
// EncryptedDEK is always the wrapped value; the vault never receives or
// returns plaintext key material.
type DEKResult struct {
	EncryptedDEK []byte
	Err          error
}

// BatchKeyVault is an optional optimized extension implemented by the
// built-in adapters. The base KeyVault remains small so custom adapters can
// provide a serial fallback while they are being upgraded.
type BatchKeyVault interface {
	KeyVault

	// GetDEKs retrieves wrapped DEKs for multiple URNs. Missing or malformed
	// individual items are reported in the corresponding DEKResult.
	GetDEKs(ctx context.Context, urns []string) (map[string]DEKResult, error)
}

// ConditionalKeyVault provides an atomic create-if-absent operation for
// generated DEKs. It prevents concurrent requests from replacing one
// another's newly generated key.
type ConditionalKeyVault interface {
	KeyVault

	// PutIfAbsent stores a wrapped DEK only when the URN has no value and
	// returns true when this call created it. ErrKeyDestroyed is returned for
	// a URN that has already been shredded.
	PutIfAbsent(ctx context.Context, urn string, encryptedDEK []byte) (created bool, err error)
}
