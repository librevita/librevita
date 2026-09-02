package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"librevita.org/pkg/ident"

	"librevita.org/ent"
	"librevita.org/ent/clinic"
	"librevita.org/ent/patient"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
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

	// Test Ent Patient entity operations (AL-FLE)
	clinicID := ident.ClinicID(uuid.New())
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

	assert.False(t, created.ID.IsZero())
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

func TestSeedInitialDataAndAuditTriggers(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "seed_test.db")

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

	ctx := context.Background()
	require.NoError(t, Migrate(ctx, store.SQL(), logger))

	// 1. SeedInitialData
	require.NoError(t, SeedInitialData(ctx, store.Ent()))
	count, err := store.Ent().IdentifierSystem.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, count)

	// Idempotent
	require.NoError(t, SeedInitialData(ctx, store.Ent()))
	count2, err := store.Ent().IdentifierSystem.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, count2)

	// Nil client check
	assert.NoError(t, SeedInitialData(ctx, nil))

	// 2. EnsureAuditTriggers
	require.NoError(t, EnsureAuditTriggers(ctx, store.SQL(), config.DriverSQLite))
	assert.NoError(t, EnsureAuditTriggers(ctx, nil, config.DriverSQLite))

	// Insert audit row
	res, err := store.SQL().ExecContext(ctx,
		`INSERT INTO audit_log (created_at, actor_name, action, resource, result, signature) 
		 VALUES (datetime('now'), 'admin', 'test.action', 'clinic', 'success', 'sig123')`,
	)
	require.NoError(t, err)
	insertID, err := res.LastInsertId()
	require.NoError(t, err)

	// Update must fail due to trigger
	_, err = store.SQL().ExecContext(ctx, `UPDATE audit_log SET actor_name = 'hacker' WHERE id = ?`, insertID)
	assert.Error(t, err)

	// Delete must fail due to trigger
	_, err = store.SQL().ExecContext(ctx, `DELETE FROM audit_log WHERE id = ?`, insertID)
	assert.Error(t, err)
}

func TestWithTxCommitAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tx_test.db")

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

	ctx := context.Background()
	require.NoError(t, Migrate(ctx, store.SQL(), logger))

	clinicID := ident.New[ident.ClinicID]()

	// 1. Commit on success
	err = WithTx(ctx, store.Ent(), func(tx *ent.Tx) error {
		return tx.Clinic.Create().
			SetID(clinicID).
			SetSlug("clinic-tx-commit").
			SetName("Tx Commit").
			SetCountry("BR").
			SetTimezone("UTC").
			Exec(ctx)
	})
	require.NoError(t, err)

	exists, err := store.Ent().Clinic.Query().Where(clinic.IDEQ(clinicID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists)

	// 2. Rollback on error
	clinicIDRollback := ident.New[ident.ClinicID]()
	err = WithTx(ctx, store.Ent(), func(tx *ent.Tx) error {
		_ = tx.Clinic.Create().
			SetID(clinicIDRollback).
			SetSlug("clinic-tx-rollback").
			SetName("Tx Rollback").
			SetCountry("BR").
			SetTimezone("UTC").
			Exec(ctx)
		return assert.AnError
	})
	assert.ErrorIs(t, err, assert.AnError)

	existsRollback, err := store.Ent().Clinic.Query().Where(clinic.IDEQ(clinicIDRollback)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, existsRollback)
}

func TestDatabaseModuleLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "module_test.db")

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

	db := sqlDB(store)
	assert.NotNil(t, db)

	master := make([]byte, 32)
	hasher, err := crypto.NewMasterIndexHasher(master)
	require.NoError(t, err)
	encryptor, err := crypto.NewMasterEncryptor(master)
	require.NoError(t, err)

	client := entClient(store, hasher, encryptor, nil)
	assert.NotNil(t, client)

	lc := fxtest.NewLifecycle(t)
	registerLifecycle(lc, store, logger)

	require.NoError(t, lc.Start(context.Background()))
	require.NoError(t, lc.Stop(context.Background()))
	assert.NotNil(t, Module)

	// Unknown driver error
	badCfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: "unknown_driver",
		},
	}
	_, err = NewStore(badCfg, logger)
	assert.Error(t, err)

	// ensureParentDir edge cases
	assert.NoError(t, ensureParentDir("."))
	assert.NoError(t, ensureParentDir(""))
}

func TestWithTxRollbackOnErrorAndPanic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tx_test.db")

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: config.DriverSQLite,
			SQLite: config.SQLiteConfig{Path: dbPath},
		},
	}
	store, err := NewStore(cfg, log.Nop())
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, Migrate(ctx, store.SQL(), log.Nop()))
	require.NoError(t, store.Ent().Schema.Create(ctx))

	// 1. WithTx returns error and rolls back
	err = WithTx(ctx, store.Ent(), func(tx *ent.Tx) error {
		return errors.New("deliberate tx failure")
	})
	assert.Error(t, err)

	// 2. WithTx rolls back on panic
	assert.Panics(t, func() {
		_ = WithTx(ctx, store.Ent(), func(tx *ent.Tx) error {
			panic("deliberate tx panic")
		})
	})
}

func TestGooseLoggerAndEnsureAuditTriggers(t *testing.T) {
	logger := log.Nop()
	gl := gooseLogger{log: logger}
	gl.Printf("goose info %s", "message")
	gl.Printf("   ") // empty string branch
	gl.Fatalf("goose fatal %s", "message")

	// EnsureAuditTriggers with nil db returns nil
	err := EnsureAuditTriggers(context.Background(), nil, "unknown_driver")
	assert.NoError(t, err)

	// Store.Close with nil db and ent returns nil
	emptyStore := &Store{}
	assert.NoError(t, emptyStore.Close())

	// ensureParentDir creates directory
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "sub", "db.sqlite")
	assert.NoError(t, ensureParentDir(nestedPath))
}

