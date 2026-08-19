package vault

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"librevita.org/internal/core/crypto"
)

func TestBBoltVaultDirectOperations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "keys.db")

	v, err := NewBBoltVault(dbPath)
	if err != nil {
		t.Fatalf("NewBBoltVault: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })

	ctx := context.Background()
	patientURN := "urn:librevita:patient:018f7654-3210-7000-8000-000000000001"
	encDEK := []byte("encrypted-dek-payload-32-bytes!!")

	// Get before Put should return crypto.ErrKeyNotFound
	if _, err := v.GetDEK(ctx, patientURN); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("GetDEK before Put = %v, want %v", err, crypto.ErrKeyNotFound)
	}

	// Put DEK
	if err := v.PutDEK(ctx, patientURN, encDEK); err != nil {
		t.Fatalf("PutDEK: %v", err)
	}

	// Get DEK
	got, err := v.GetDEK(ctx, patientURN)
	if err != nil {
		t.Fatalf("GetDEK: %v", err)
	}
	if !bytes.Equal(got, encDEK) {
		t.Fatalf("GetDEK = %q, want %q", got, encDEK)
	}

	// Delete DEK (Crypto-Shredding)
	if err := v.DeleteDEK(ctx, patientURN); err != nil {
		t.Fatalf("DeleteDEK: %v", err)
	}

	// Get after Delete should return crypto.ErrKeyNotFound
	if _, err := v.GetDEK(ctx, patientURN); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("GetDEK after Delete = %v, want %v", err, crypto.ErrKeyNotFound)
	}
}
