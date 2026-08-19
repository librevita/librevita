package crypto

import (
	"context"
	"errors"
)

var (
	// ErrKeyNotFound indicates the requested patient DEK does not exist in the vault.
	ErrKeyNotFound = errors.New("crypto: patient DEK not found in vault")

	// ErrVaultClosed indicates operations were attempted on a closed KeyVault.
	ErrVaultClosed = errors.New("crypto: key vault is closed")

	// ErrInvalidDEK indicates a malformed or invalid DEK payload.
	ErrInvalidDEK = errors.New("crypto: invalid DEK size or format")
)

// KeyVault defines the Hexagonal Port interface for storing and retrieving
// encrypted patient Data Encryption Keys (DEKs).
//
// Implementations can range from embedded Key-Value databases (e.g. bbolt)
// for single-node deployments to enterprise remote KMS/Vault solutions
// (HashiCorp Vault, AWS KMS, GCP Key Management) for multi-node clusters.
type KeyVault interface {
	// PutDEK stores the encrypted DEK bytes associated with a patient URN.
	PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error

	// GetDEK retrieves the encrypted DEK bytes for a patient URN.
	// Returns ErrKeyNotFound if no key exists for the given URN.
	GetDEK(ctx context.Context, patientURN string) ([]byte, error)

	// DeleteDEK permanently deletes the patient's DEK from storage,
	// executing instant Crypto-Shredding for GDPR/LGPD Right to be Forgotten.
	DeleteDEK(ctx context.Context, patientURN string) error

	// Close cleanly releases any vault storage resources or connections.
	Close() error
}
