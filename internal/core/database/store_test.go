package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"librevita.org/ent/patient"
	"librevita.org/internal/core/config"
)

func TestStoreSQLiteAndEntClient(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: config.DriverSQLite,
			SQLite: config.SQLiteConfig{
				Path: dbPath,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := NewStore(cfg, logger)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer store.Close()

	if store.Driver() != config.DriverSQLite {
		t.Errorf("Driver() = %q, want %q", store.Driver(), config.DriverSQLite)
	}
	if store.SQL() == nil {
		t.Fatal("SQL() returned nil")
	}
	if store.Ent() == nil {
		t.Fatal("Ent() returned nil")
	}

	// Apply migrations to test schema
	ctx := context.Background()
	if err := Migrate(ctx, store.SQL(), logger); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// Also create Ent schema resources in the database
	if err := store.Ent().Schema.Create(ctx); err != nil {
		t.Fatalf("Ent.Schema.Create() failed: %v", err)
	}

	// Test Ent Patient entity operations (Zero-Knowledge)
	clinicID := uuid.New()
	_, err = store.Ent().Clinic.Create().
		SetID(clinicID).
		SetName("Test Clinic").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed creating clinic in Ent: %v", err)
	}

	blindIndex := "8f48259ca9a27e7f603c4f74d0089ff2bf309f7a7d45f3a0937a0c8b21c4309a"
	encryptedPayload := []byte("encrypted-patient-demographics-payload")
	nonce := make([]byte, 24)
	for i := range nonce {
		nonce[i] = byte(i)
	}

	created, err := store.Ent().Patient.Create().
		SetClinicID(clinicID).
		SetBlindIndex(blindIndex).
		SetEncryptedPayload(encryptedPayload).
		SetNonce(nonce).
		SetStatus("active").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed creating patient in Ent: %v", err)
	}

	if created.ID == uuid.Nil {
		t.Errorf("created.ID is Nil, expected generated UUID")
	}
	if created.BlindIndex != blindIndex {
		t.Errorf("created.BlindIndex = %q, want %q", created.BlindIndex, blindIndex)
	}

	// Query by exact blind index within clinic
	found, err := store.Ent().Patient.Query().
		Where(
			patient.ClinicID(clinicID),
			patient.BlindIndex(blindIndex),
		).
		Only(ctx)
	if err != nil {
		t.Fatalf("failed querying patient by blind index: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("found.ID = %v, want %v", found.ID, created.ID)
	}
}
