package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/enttest"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/pkg/ident"
)

func setupTestClinicDB(t *testing.T) (*record.Client, context.Context) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:ent_clinic_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(record.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	return client, context.Background()
}

func TestClinicRepository(t *testing.T) {
	client, ctx := setupTestClinicDB(t)
	repo := repository.NewClinicRepository(client)

	cid := ident.New[ident.ClinicID]()
	c := &model.Clinic{
		ID:         cid,
		Slug:       "clinic-alpha",
		Name:       "Clinic Alpha",
		TaxID:      "12345678901",
		Phone:      "+5511999990000",
		Email:      "alpha@example.org",
		Street:     "Rua das Flores 123",
		City:       "São Paulo",
		State:      "SP",
		PostalCode: "01000-000",
		Country:    "BR",
		Timezone:   "America/Sao_Paulo",
	}

	// 1. CreateShell
	created, err := repo.CreateShell(ctx, c)
	require.NoError(t, err)
	assert.Equal(t, cid, created.ID)
	assert.Equal(t, "clinic-alpha", created.Slug)

	// Duplicate slug create
	_, err = repo.CreateShell(ctx, c)
	assert.Error(t, err)

	// 2. GetByID
	fetched, err := repo.GetByID(ctx, cid)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "Clinic Alpha", fetched.Name)

	// GetByID not found returns nil, nil
	notFound, err := repo.GetByID(ctx, ident.New[ident.ClinicID]())
	require.NoError(t, err)
	assert.Nil(t, notFound)

	// 3. GetBySlug
	bySlug, err := repo.GetBySlug(ctx, "clinic-alpha")
	require.NoError(t, err)
	require.NotNil(t, bySlug)
	assert.Equal(t, cid, bySlug.ID)

	// GetBySlug not found
	bySlugNotFound, err := repo.GetBySlug(ctx, "unknown-slug")
	require.NoError(t, err)
	assert.Nil(t, bySlugNotFound)

	// 4. MarkOnboarded
	now := time.Now().UTC()
	err = repo.MarkOnboarded(ctx, cid, now)
	require.NoError(t, err)

	onboardedClinic, err := repo.GetByID(ctx, cid)
	require.NoError(t, err)
	require.NotNil(t, onboardedClinic.OnboardedAt)

	// 5. List
	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestPlatformUserRepository(t *testing.T) {
	client, ctx := setupTestClinicDB(t)
	repo := repository.NewPlatformUserRepository(client)

	// 1. Count initially 0
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	opID := ident.New[ident.PlatformUserID]()
	op := &repository.PlatformUser{
		ID:           opID,
		Email:        "op@librevita.org",
		PasswordHash: "passhash",
		DisplayName:  "Operator One",
		Active:       true,
	}

	// 2. Create
	created, err := repo.Create(ctx, op)
	require.NoError(t, err)
	assert.Equal(t, opID, created.ID)

	// Duplicate create
	_, err = repo.Create(ctx, op)
	assert.Error(t, err)

	// 3. Count after create
	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// 4. GetByID
	fetched, err := repo.GetByID(ctx, opID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "Operator One", fetched.DisplayName)

	// GetByID not found
	notFound, err := repo.GetByID(ctx, ident.New[ident.PlatformUserID]())
	require.NoError(t, err)
	assert.Nil(t, notFound)

	// 5. GetByEmail
	byEmail, err := repo.GetByEmail(ctx, "op@librevita.org")
	require.NoError(t, err)
	require.NotNil(t, byEmail)
	assert.Equal(t, opID, byEmail.ID)

	// GetByEmail not found
	byEmailNotFound, err := repo.GetByEmail(ctx, "unknown@librevita.org")
	require.NoError(t, err)
	assert.Nil(t, byEmailNotFound)
}
