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
	return v.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.Put([]byte(patientURN), encryptedDEK)
	})
}

// GetDEK retrieves the encrypted DEK bytes for patientURN.
// Returns crypto.ErrKeyNotFound if the key does not exist.
func (v *BBoltVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	var val []byte
	err := v.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		data := b.Get([]byte(patientURN))
		if data == nil {
			return crypto.ErrKeyNotFound
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

// DeleteDEK removes the patient's DEK from storage, performing instant Crypto-Shredding.
func (v *BBoltVault) DeleteDEK(ctx context.Context, patientURN string) error {
	return v.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.Delete([]byte(patientURN))
	})
}

// Close closes the underlying bbolt database.
func (v *BBoltVault) Close() error {
	return v.db.Close()
}
