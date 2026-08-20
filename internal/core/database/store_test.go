package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	defer store.Close()

	assert.Equal(t, config.DriverSQLite, store.Driver())
	require.NotNil(t, store.SQL())
	require.NotNil(t, store.Ent())

	// Apply migrations to test schema
	ctx := context.Background()
	err = Migrate(ctx, store.SQL(), logger)
	require.NoError(t, err)

	// Also create Ent schema resources in the database
	err = store.Ent().Schema.Create(ctx)
	require.NoError(t, err)

	// Test Ent Patient entity operations (Zero-Knowledge)
	clinicID := uuid.New()
	_, err = store.Ent().Clinic.Create().
		SetID(clinicID).
		SetName("Test Clinic").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(ctx)
	require.NoError(t, err)

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
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, blindIndex, created.BlindIndex)

	// Query by exact blind index within clinic
	found, err := store.Ent().Patient.Query().
		Where(
			patient.ClinicID(clinicID),
			patient.BlindIndex(blindIndex),
		).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}
