// Package vault provides infrastructure adapters for key storage.
// It implements the crypto.KeyVault port declared by internal/core/crypto.
package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.etcd.io/bbolt"

	"librevita.org/internal/core/crypto"
)

var bucketName = []byte("patient_deks")

// BBoltVault implements crypto.KeyVault using an embedded bbolt database.
type BBoltVault struct {
	db *bbolt.DB
}

// NewBBoltVault creates or opens a bbolt KeyVault database at dbPath.
func NewBBoltVault(dbPath string) (*BBoltVault, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("vault: mkdir: %w", err)
	}

	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: open bbolt: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("vault: init bucket: %w", err)
	}

	return &BBoltVault{db: db}, nil
}

// PutDEK stores the encrypted DEK bytes indexed by patientURN.
func (v *BBoltVault) PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return v.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		if crypto.IsDestroyedDEK(b.Get([]byte(patientURN))) {
			return crypto.ErrKeyDestroyed
		}
		return b.Put([]byte(patientURN), encryptedDEK)
	})
}

// GetDEK retrieves the encrypted DEK bytes for patientURN.
// Returns crypto.ErrKeyNotFound if the key does not exist.
func (v *BBoltVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var val []byte
	err := v.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		data := b.Get([]byte(patientURN))
		if data == nil {
			return crypto.ErrKeyNotFound
		}
		if crypto.IsDestroyedDEK(data) {
			return crypto.ErrKeyDestroyed
		}
		val = make([]byte, len(data))
		copy(val, data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

// GetDEKs retrieves multiple wrapped DEKs in one bbolt read transaction.
func (v *BBoltVault) GetDEKs(ctx context.Context, patientURNs []string) (map[string]crypto.DEKResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unique := uniqueURNs(patientURNs)
	results := make(map[string]crypto.DEKResult, len(unique))
	err := v.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		for _, patientURN := range unique {
			if err := ctx.Err(); err != nil {
				return err
			}
			data := b.Get([]byte(patientURN))
			switch {
			case data == nil:
				results[patientURN] = crypto.DEKResult{Err: crypto.ErrKeyNotFound}
			case crypto.IsDestroyedDEK(data):
				results[patientURN] = crypto.DEKResult{Err: crypto.ErrKeyDestroyed}
			default:
				value := make([]byte, len(data))
				copy(value, data)
				results[patientURN] = crypto.DEKResult{EncryptedDEK: value}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// PutIfAbsent stores a wrapped DEK without replacing an existing value.
func (v *BBoltVault) PutIfAbsent(ctx context.Context, patientURN string, encryptedDEK []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	created := false
	err := v.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		existing := b.Get([]byte(patientURN))
		if existing != nil {
			if crypto.IsDestroyedDEK(existing) {
				return crypto.ErrKeyDestroyed
			}
			return nil
		}
		if err := b.Put([]byte(patientURN), encryptedDEK); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// DeleteDEK replaces the patient's DEK with a terminal tombstone, performing
// instant Crypto-Shredding while preventing accidental recreation.
func (v *BBoltVault) DeleteDEK(ctx context.Context, patientURN string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return v.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.Put([]byte(patientURN), crypto.DestroyedDEKMarker())
	})
}

// Close closes the underlying bbolt database.
func (v *BBoltVault) Close() error {
	return v.db.Close()
}
