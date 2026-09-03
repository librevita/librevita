package usecase_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/keystore"
	"librevita.org/internal/database/record"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/clinic/usecase"
	"librevita.org/pkg/log"
)

func setupPlatformEnv(t *testing.T) (*usecase.PlatformService, *record.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "platform.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, database.Migrate(context.Background(), db, log.Nop()))

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := record.NewClient(record.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	masterKey, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	require.NoError(t, err)

	userRepo := clinicrepo.NewPlatformUserRepository(client)
	clinicRepo := clinicrepo.NewClinicRepository(client)

	svc := usecase.NewPlatformService(userRepo, clinicRepo, masterKey)
	return svc, client
}

func TestPlatformServiceLifecycle(t *testing.T) {
	svc, _ := setupPlatformEnv(t)
	ctx := context.Background()

	// 1. Initial state: HasOperators is false
	has, err := svc.HasOperators(ctx)
	require.NoError(t, err)
	assert.False(t, has)

	// 2. Bootstrap validation errors
	_, _, err = svc.Bootstrap(ctx, "", "admin@example.org", "password123")
	assert.Error(t, err)
	_, _, err = svc.Bootstrap(ctx, "Admin", "invalid-email", "password123")
	assert.Error(t, err)
	_, _, err = svc.Bootstrap(ctx, "Admin", "admin@example.org", "short")
	assert.Error(t, err)

	// 3. Bootstrap success
	p, _, err := svc.Bootstrap(ctx, "Platform Master", "master@librevita.org", "MasterPass123!")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "master@librevita.org", p.Email)
	assert.True(t, p.Platform)

	// 4. HasOperators is now true
	has, err = svc.HasOperators(ctx)
	require.NoError(t, err)
	assert.True(t, has)

	// 5. Bootstrap second time returns ErrPlatformExists
	_, _, err = svc.Bootstrap(ctx, "Second Master", "second@librevita.org", "SecondPass123!")
	assert.ErrorIs(t, err, usecase.ErrPlatformExists)

	// 6. Login success
	loggedIn, err := svc.Login(ctx, "master@librevita.org", "MasterPass123!")
	require.NoError(t, err)
	assert.Equal(t, p.ID, loggedIn.ID)
	assert.True(t, loggedIn.Platform)

	// 7. Login with wrong password
	_, err = svc.Login(ctx, "master@librevita.org", "WrongPassword!")
	assert.ErrorIs(t, err, usecase.ErrInvalidPlatformCred)

	// 8. Login with non-existent email
	_, err = svc.Login(ctx, "missing@librevita.org", "AnyPassword123!")
	assert.ErrorIs(t, err, usecase.ErrInvalidPlatformCred)

	// 9. Provision clinic shell
	clinic, err := svc.Provision(ctx, usecase.ProvisionInput{
		Name:     "Clinica Alfa",
		Slug:     "clinica-alfa",
		TaxID:    "12.345.678/0001-99",
		Timezone: "America/Sao_Paulo",
	})
	require.NoError(t, err)
	require.NotNil(t, clinic)
	assert.Equal(t, "clinica-alfa", clinic.Slug)
	assert.Equal(t, "Clinica Alfa", clinic.Name)

	// 10. Provision with invalid slug or empty name
	_, err = svc.Provision(ctx, usecase.ProvisionInput{
		Name: "Clinica Invalida",
		Slug: "INVALID_SLUG_!",
	})
	assert.ErrorIs(t, err, usecase.ErrInvalidSlug)

	_, err = svc.Provision(ctx, usecase.ProvisionInput{
		Name: "",
		Slug: "valid-slug",
	})
	assert.Error(t, err)

	// 11. ListClinics
	clinics, err := svc.ListClinics(ctx)
	require.NoError(t, err)
	assert.Len(t, clinics, 1)
}
