package repository_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/enttest"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/normalize"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/internal/domain/patient/repository"
)

const testKeyB64 = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=" // gitleaks:allow

func strPtr(s string) *string { return &s }

func setupTestRepository(t *testing.T) (patientmodel.PatientRepository, *ent.Client, crypto.Hasher, uuid.UUID, uuid.UUID) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:ent?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	key, err := base64.StdEncoding.DecodeString(testKeyB64)
	require.NoError(t, err)

	hasher, err := crypto.NewHasher(key)
	require.NoError(t, err)

	encryptor, err := crypto.NewEncryptor(key)
	require.NoError(t, err)

	// Register compile-time typed FLE hook and context-aware decryption interceptor
	client.Use(ent.FLEMutationHook(hasher, encryptor))
	client.Intercept(ent.FLEDecryptionInterceptor(encryptor))

	repo := repository.NewPatientRepository(client)

	// Create clinic, role, and user
	clinicID := uuid.New()
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("clinica-central").
		SetName("Clínica Central").
		Save(context.Background())
	require.NoError(t, err)

	roleID := uuid.New()
	_, err = client.Role.Create().
		SetID(roleID).
		SetClinicID(clinicID).
		SetName("Doctor").
		Save(context.Background())
	require.NoError(t, err)

	userID := uuid.New()
	_, err = client.User.Create().
		SetID(userID).
		SetClinicID(clinicID).
		SetRoleID(roleID).
		SetEmail("dr.silva@example.org").
		SetDisplayName("Dr. Silva").
		SetPasswordHash("argon2id$test").
		Save(context.Background())
	require.NoError(t, err)

	return repo, client, hasher, clinicID, userID
}

func TestPatientRepository_CRUD(t *testing.T) {
	repo, client, hasher, clinicID, userID := setupTestRepository(t)
	ctx := context.Background()

	patientID := uuid.New()
	p := patientmodel.Patient{
		ID:          patientID,
		ClinicID:    clinicID,
		DisplayName: "Maria Joana",
		BirthDate:   strPtr("1988-05-20"),
		Sex:         patientmodel.SexFemale,
		Email:       strPtr("maria@example.org"),
		Phone:       strPtr("+55 11 98888-7777"),
		Street:      strPtr("Av. Paulista 1000"),
		City:        strPtr("São Paulo"),
		State:       strPtr("SP"),
		PostalCode:  strPtr("01310-100"),
		Notes:       strPtr("Paciente alérgica a dipirona"),
		Status:      patientmodel.PatientStatusActive,
		CreatedBy:   &userID,
	}

	// 1. Create
	created, err := repo.Create(ctx, p)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, patientID, created.ID)
	assert.Equal(t, clinicID, created.ClinicID)
	assert.Equal(t, "Maria Joana", created.DisplayName)
	assert.Equal(t, "maria@example.org", *created.Email)
	assert.Equal(t, patientmodel.PatientStatusActive, created.Status)

	// Verify transparent automatic blind index computation in Ent row
	rawRow, err := client.Patient.Get(ctx, patientID)
	require.NoError(t, err)
	assert.Equal(t, "Maria Joana", rawRow.DisplayName) // Transparently decrypted by interceptor!

	expectedEmailBlind, _ := hasher.BlindIndex("patient.email", normalize.Email("maria@example.org"))
	expectedPhoneBlind, _ := hasher.BlindIndex("patient.phone", normalize.Phone("+55 11 98888-7777"))
	assert.Equal(t, expectedEmailBlind, rawRow.EmailBlindIndex)
	assert.Equal(t, expectedPhoneBlind, rawRow.PhoneBlindIndex)

	// Verify tokenized search n-grams generated in JSON column
	expectedTokens := normalize.NameTokens("Maria Joana")
	require.NotEmpty(t, rawRow.DisplayNameTokenIndex)
	require.Equal(t, len(expectedTokens), len(rawRow.DisplayNameTokenIndex))
	for _, tok := range expectedTokens {
		h, _ := hasher.BlindIndex("patient.token", tok)
		assert.Contains(t, rawRow.DisplayNameTokenIndex, h)
	}

	optimized, ok := repo.(patientmodel.PatientQueryRepository)
	require.True(t, ok)
	nameHash, err := hasher.BlindIndex("patient.token", "maria")
	require.NoError(t, err)
	candidates, totalCandidates, err := optimized.ListCandidates(ctx, clinicID, nil, []string{nameHash}, "", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, totalCandidates)
	require.Len(t, candidates, 1)
	assert.Equal(t, patientID, candidates[0].ID)
	hydrated, err := optimized.GetMany(ctx, clinicID, []uuid.UUID{patientID})
	require.NoError(t, err)
	require.Len(t, hydrated, 1)
	assert.Equal(t, "Maria Joana", hydrated[0].DisplayName)

	// 2. Get
	fetched, err := repo.Get(ctx, clinicID, patientID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, patientID, fetched.ID)
	assert.Equal(t, "Maria Joana", fetched.DisplayName)
	assert.Equal(t, "1988-05-20", *fetched.BirthDate)
	assert.Equal(t, patientmodel.SexFemale, fetched.Sex)
	assert.Equal(t, "maria@example.org", *fetched.Email)
	assert.Equal(t, "+55 11 98888-7777", *fetched.Phone)

	// 3. GetWithCreator
	withCreator, err := repo.GetWithCreator(ctx, clinicID, patientID)
	require.NoError(t, err)
	require.NotNil(t, withCreator)
	assert.Equal(t, patientID, withCreator.ID)
	require.NotNil(t, withCreator.CreatorName)
	assert.Equal(t, "Dr. Silva", *withCreator.CreatorName)
	require.NotNil(t, withCreator.CreatorEmail)
	assert.Equal(t, "dr.silva@example.org", *withCreator.CreatorEmail)

	// 4. Update
	p.DisplayName = "Maria Joana da Silva"
	p.Email = strPtr("maria.silva@example.org")
	p.Status = patientmodel.PatientStatusInactive

	updated, err := repo.Update(ctx, p)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Maria Joana da Silva", updated.DisplayName)
	assert.Equal(t, "maria.silva@example.org", *updated.Email)
	assert.Equal(t, patientmodel.PatientStatusInactive, updated.Status)

	// 5. ListByClinicAndStatus
	listActive, err := repo.ListByClinicAndStatus(ctx, clinicID, &p.Status)
	require.NoError(t, err)
	assert.Len(t, listActive, 1)
	assert.Equal(t, "Maria Joana da Silva", listActive[0].DisplayName)

	// 6. BulkSetStatus
	count, err := repo.BulkSetStatus(ctx, clinicID, []uuid.UUID{patientID}, patientmodel.PatientStatusArchived)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// 7. Count
	total, err := repo.Count(ctx, clinicID)
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	deleter, ok := repo.(patientmodel.PatientDeletionRepository)
	require.True(t, ok)
	require.NoError(t, deleter.DeleteAggregate(ctx, clinicID, patientID))
	require.NoError(t, deleter.DeleteAggregate(ctx, clinicID, patientID))
	_, err = repo.Get(ctx, clinicID, patientID)
	assert.ErrorIs(t, err, patientmodel.ErrNotFound)
}
