package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/enttest"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/internal/domain/identifier/repository"
	"librevita.org/pkg/ident"
)

func setupTestIdentifierDB(t *testing.T) (*ent.Client, ident.ClinicID, ident.PatientID, context.Context) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:ent_ident_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	ctx := context.Background()
	clinicID := ident.New[ident.ClinicID]()
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("clinica-ident").
		SetName("Clínica Ident").
		Save(ctx)
	require.NoError(t, err)

	patID := ident.New[ident.PatientID]()
	_, err = client.Patient.Create().
		SetID(patID).
		SetClinicID(clinicID).
		SetDisplayName("Paciente Ident").
		SetEmail("paciente@ident.test").
		SetPhone("+5511999998888").
		Save(ctx)
	require.NoError(t, err)

	return client, clinicID, patID, ctx
}

func TestSystemRepository(t *testing.T) {
	client, _, _, ctx := setupTestIdentifierDB(t)
	repo := repository.NewSystemRepository(client)

	// 1. SeedDefaults
	err := repo.SeedDefaults(ctx)
	require.NoError(t, err)

	// Repeat SeedDefaults is idempotent
	err = repo.SeedDefaults(ctx)
	require.NoError(t, err)

	// 2. ListAll & ListActive
	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(all), 4)

	active, err := repo.ListActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(all), len(active))

	// 3. GetBySystem
	bySys, err := repo.GetBySystem(ctx, all[0].System)
	require.NoError(t, err)
	assert.Equal(t, all[0].ID, bySys.ID)

	// GetBySystem not found
	_, err = repo.GetBySystem(ctx, "nonexistent-system")
	assert.ErrorIs(t, err, identifiermodel.ErrSystemNotFound)

	// 4. GetByID
	byID, err := repo.GetByID(ctx, all[0].ID)
	require.NoError(t, err)
	assert.Equal(t, all[0].System, byID.System)

	// GetByID not found
	_, err = repo.GetByID(ctx, ident.New[ident.IdentifierSystemID]())
	assert.ErrorIs(t, err, identifiermodel.ErrSystemNotFound)

	// 5. Create Custom System
	sysID := ident.New[ident.IdentifierSystemID]()
	customSys := &identifiermodel.IdentifierSystem{
		ID:             sysID,
		System:         "custom:test",
		DisplayName:    "Custom System",
		Pattern:        "[0-9]{5}",
		Transform:      identifiermodel.TransformDigits,
		CheckAlgorithm: identifiermodel.CheckNone,
		Active:         true,
	}
	created, err := repo.Create(ctx, customSys)
	require.NoError(t, err)
	assert.Equal(t, sysID, created.ID)

	// Duplicate create
	_, err = repo.Create(ctx, customSys)
	assert.ErrorIs(t, err, identifiermodel.ErrDuplicate)

	// 6. Update
	created.DisplayName = "Updated Custom System"
	updated, err := repo.Update(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Custom System", updated.DisplayName)

	// Update not found
	fakeSys := &identifiermodel.IdentifierSystem{ID: ident.New[ident.IdentifierSystemID](), System: "fake:sys", DisplayName: "Fake System", Pattern: "[0-9]+", Transform: identifiermodel.TransformDigits, CheckAlgorithm: identifiermodel.CheckNone}
	_, err = repo.Update(ctx, fakeSys)
	assert.ErrorIs(t, err, identifiermodel.ErrSystemNotFound)

	// 7. SetActive
	err = repo.SetActive(ctx, sysID, false)
	require.NoError(t, err)

	deactivated, err := repo.GetByID(ctx, sysID)
	require.NoError(t, err)
	assert.False(t, deactivated.Active)

	// SetActive not found
	err = repo.SetActive(ctx, ident.New[ident.IdentifierSystemID](), true)
	assert.ErrorIs(t, err, identifiermodel.ErrSystemNotFound)
}

func TestIdentifierRepository(t *testing.T) {
	client, clinicID, patID, ctx := setupTestIdentifierDB(t)
	repo := repository.NewIdentifierRepository(client)
	sysRepo := repository.NewSystemRepository(client)
	require.NoError(t, sysRepo.SeedDefaults(ctx))

	// 1. PatientExists
	exists, err := repo.PatientExists(ctx, clinicID, patID)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = repo.PatientExists(ctx, clinicID, ident.New[ident.PatientID]())
	require.NoError(t, err)
	assert.False(t, exists)

	// 2. AllowsSystem (opt-in link)
	cpfURN := "urn:librevita:id:br:cpf"
	sys, err := sysRepo.GetBySystem(ctx, cpfURN)
	require.NoError(t, err)

	optID := ident.New[ident.ClinicIdentifierSystemID]()
	_, err = client.ClinicIdentifierSystem.Create().
		SetID(optID).
		SetClinicID(clinicID).
		SetIdentifierSystemID(sys.ID).
		Save(ctx)
	require.NoError(t, err)

	allowed, err := repo.AllowsSystem(ctx, clinicID, cpfURN)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = repo.AllowsSystem(ctx, clinicID, "urn:librevita:id:pt:nif")
	require.NoError(t, err)
	assert.False(t, allowed)

	// 3. Add Identifier
	idID := ident.New[ident.PatientIdentifierID]()
	rec := identifiermodel.IdentifierRecord{
		ID:              idID,
		ClinicID:        clinicID,
		PatientID:       patID,
		System:          cpfURN,
		ValueCiphertext: []byte("encrypted-cpf"),
		BlindIndex:      "blind-index-cpf-123",
	}

	created, err := repo.Add(ctx, rec)
	require.NoError(t, err)
	assert.Equal(t, idID, created.ID)

	// Duplicate add
	_, err = repo.Add(ctx, rec)
	assert.ErrorIs(t, err, identifiermodel.ErrDuplicate)

	// 4. FindByBlindIndex
	found, err := repo.FindByBlindIndex(ctx, clinicID, "blind-index-cpf-123")
	require.NoError(t, err)
	assert.Equal(t, idID, found.ID)

	// FindByBlindIndex not found
	_, err = repo.FindByBlindIndex(ctx, clinicID, "unknown-blind-index")
	assert.ErrorIs(t, err, identifiermodel.ErrNotFound)

	// 5. ListByPatient
	list, err := repo.ListByPatient(ctx, patID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// ListByPatients
	multi, err := repo.ListByPatients(ctx, []ident.PatientID{patID})
	require.NoError(t, err)
	assert.Len(t, multi, 1)

	emptyMulti, err := repo.ListByPatients(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, emptyMulti)

	// 6. Remove
	err = repo.Remove(ctx, patID, idID)
	require.NoError(t, err)

	// Remove not found
	err = repo.Remove(ctx, patID, idID)
	assert.ErrorIs(t, err, identifiermodel.ErrNotFound)
}
