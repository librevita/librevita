package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/ent/patient"
	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
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

	logger := log.Nop()
	store, err := NewStore(cfg, logger)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

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
		SetSlug("test-clinic").
		SetName("Test Clinic").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(ctx)
	require.NoError(t, err)

	created, err := store.Ent().Patient.Create().
		SetClinicID(clinicID).
		SetDisplayName("Maria Teste").
		SetPhone("+55 11 99999-8888").
		SetEmail("maria@example.org").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, "Maria Teste", created.DisplayName)

	// Query by display name within clinic
	found, err := store.Ent().Patient.Query().
		Where(
			patient.ClinicID(clinicID),
			patient.DisplayNameEQ("Maria Teste"),
		).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}
