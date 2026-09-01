package crypto

import (
	"bytes"
	"context"

	"github.com/cockroachdb/errors"
)

var (
	// ErrKeyNotFound indicates the requested DEK does not exist in the keystore.
	ErrKeyNotFound = errors.New("crypto: dek not found in keystore")

	// ErrKeyDestroyed indicates that the key was permanently shredded and must
	// never be recreated for the same URN.
	ErrKeyDestroyed = errors.New("crypto: dek permanently destroyed")

	// ErrKeyStoreClosed indicates operations were attempted on a closed KeyStore.
	ErrKeyStoreClosed = errors.New("crypto: keystore is closed")

	// ErrInvalidDEK indicates a malformed or invalid DEK payload.
	ErrInvalidDEK = errors.New("crypto: invalid DEK size or format")
)

var destroyedDEKMarker = []byte{0x00, 'L', 'V', '-', 'D', 'E', 'S', 'T', 'R', 'O', 'Y', 'E', 'D'}

// DestroyedDEKMarker returns a copy of the non-secret tombstone stored by
// keystore adapters after a DEK is shredded.
func DestroyedDEKMarker() []byte {
	return append([]byte(nil), destroyedDEKMarker...)
}

// IsDestroyedDEK reports whether a keystore value is a terminal shredding
// tombstone rather than an encrypted DEK.
func IsDestroyedDEK(value []byte) bool {
	return bytes.Equal(value, destroyedDEKMarker)
}

// KeyStore is the port for storing and retrieving wrapped Clinic and Patient
// DEKs. Implementations range from embedded bbolt to NATS, etcd, or
// HashiCorp Vault / OpenBao KV v2.
type KeyStore interface {
	PutDEK(ctx context.Context, urn string, encryptedDEK []byte) error
	GetDEK(ctx context.Context, urn string) ([]byte, error)
	DeleteDEK(ctx context.Context, urn string) error
	Close() error
}

// DEKResult is the result for one item returned by a batch keystore lookup.
// EncryptedDEK is always the wrapped value; the store never receives or
// returns plaintext key material.
type DEKResult struct {
	EncryptedDEK []byte
	Err          error
}

// BatchKeyStore is an optional optimized extension implemented by the
// built-in adapters. The base KeyStore remains small so custom adapters can
// provide a serial fallback while they are being upgraded.
type BatchKeyStore interface {
	KeyStore

	GetDEKs(ctx context.Context, urns []string) (map[string]DEKResult, error)
}

// ConditionalKeyStore provides an atomic create-if-absent operation for
// generated DEKs. It prevents concurrent requests from replacing one
// another's newly generated key.
type ConditionalKeyStore interface {
	KeyStore

	PutIfAbsent(ctx context.Context, urn string, encryptedDEK []byte) (created bool, err error)
}
