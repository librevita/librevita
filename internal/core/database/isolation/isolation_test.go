package isolation_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/enttest"
	"librevita.org/ent/user"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/database/isolation"
)

func TestCrossClinicUsersAndFLE(t *testing.T) {
	db, err := sql.Open("sqlite", "file:isolation?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	master := make([]byte, 32)
	_, err = rand.Read(master)
	require.NoError(t, err)
	hasher, err := crypto.NewMasterIndexHasher(master)
	require.NoError(t, err)
	encDefault, err := crypto.NewMasterEncryptor(master)
	require.NoError(t, err)

	client.Use(isolation.MutationHook())
	client.Use(ent.FLEMutationHook(hasher, encDefault))
	client.Intercept(isolation.QueryInterceptor())
	client.Intercept(ent.FLEDecryptionInterceptor(encDefault))

	seed := clinicctx.WithSkipIsolation(context.Background())
	norteID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")
	sulID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a2")
	_, err = client.Clinic.Create().SetID(norteID).SetSlug("norte").SetName("Norte").SetCountry("BR").SetTimezone("America/Sao_Paulo").Save(seed)
	require.NoError(t, err)
	_, err = client.Clinic.Create().SetID(sulID).SetSlug("sul").SetName("Sul").SetCountry("BR").SetTimezone("America/Sao_Paulo").Save(seed)
	require.NoError(t, err)

	keyA := make([]byte, 32)
	_, err = rand.Read(keyA)
	require.NoError(t, err)
	keyB := make([]byte, 32)
	_, err = rand.Read(keyB)
	require.NoError(t, err)
	encA, err := crypto.NewClinicEncryptor(keyA)
	require.NoError(t, err)
	encB, err := crypto.NewClinicEncryptor(keyB)
	require.NoError(t, err)
	hashA, err := crypto.NewHasherFromDEK(keyA)
	require.NoError(t, err)
	hashB, err := crypto.NewHasherFromDEK(keyB)
	require.NoError(t, err)

	ctxA := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{ID: norteID, Slug: "norte", Name: "Norte", Timezone: "America/Sao_Paulo"})
	ctxA = fle.WithClinicID(ctxA, norteID)
	ctxA = fle.WithEncryptor(ctxA, encA)
	ctxA = fle.WithHasher(ctxA, hashA)

	ctxB := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{ID: sulID, Slug: "sul", Name: "Sul", Timezone: "America/Sao_Paulo"})
	ctxB = fle.WithClinicID(ctxB, sulID)
	ctxB = fle.WithEncryptor(ctxB, encB)
	ctxB = fle.WithHasher(ctxB, hashB)

	roleA, err := client.Role.Create().SetClinicID(norteID).SetName("admin").SetSystem(true).Save(ctxA)
	require.NoError(t, err)
	roleB, err := client.Role.Create().SetClinicID(sulID).SetName("admin").SetSystem(true).Save(ctxB)
	require.NoError(t, err)

	userA, err := client.User.Create().
		SetEmail("shared@example.org").
		SetPasswordHash("hash-a").
		SetDisplayName("Ana Norte").
		SetRoleID(roleA.ID).
		Save(ctxA)
	require.NoError(t, err)
	userB, err := client.User.Create().
		SetEmail("shared@example.org").
		SetPasswordHash("hash-b").
		SetDisplayName("Ana Sul").
		SetRoleID(roleB.ID).
		Save(ctxB)
	require.NoError(t, err)
	assert.NotEqual(t, userA.ID, userB.ID)

	foundA, err := client.User.Query().Where(user.EmailEQ("shared@example.org")).All(ctxA)
	require.NoError(t, err)
	require.Len(t, foundA, 1)
	assert.Equal(t, userA.ID, foundA[0].ID)

	foundB, err := client.User.Query().Where(user.EmailEQ("shared@example.org")).All(ctxB)
	require.NoError(t, err)
	require.Len(t, foundB, 1)
	assert.Equal(t, userB.ID, foundB[0].ID)

	_, err = client.User.Query().All(context.Background())
	require.ErrorIs(t, err, clinicctx.ErrMissingClinic)

	name := "Maria Norte"
	pA, err := client.Patient.Create().
		SetClinicID(norteID).
		SetDisplayName(name).
		SetPhone("+55 11 91111-1111").
		SetEmail("maria.norte@example.org").
		Save(ctxA)
	require.NoError(t, err)
	gotA, err := client.Patient.Get(ctxA, pA.ID)
	require.NoError(t, err)
	assert.Equal(t, name, gotA.DisplayName)

	_, err = client.Patient.Get(ctxB, pA.ID)
	assert.True(t, ent.IsNotFound(err), "clinic B must not see clinic A's patient")

	wrongCtx := clinicctx.WithSkipIsolation(context.Background())
	wrongCtx = fle.WithEncryptor(wrongCtx, encB)
	wrongCtx = fle.WithClinicID(wrongCtx, sulID)
	wrongFetched, err := client.Patient.Get(wrongCtx, pA.ID)
	require.Error(t, err)
	assert.Nil(t, wrongFetched)
}

func TestIsolationEdgeCases(t *testing.T) {
	db, err := sql.Open("sqlite", "file:isolation_edges?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	client.Use(isolation.MutationHook())
	client.Intercept(isolation.QueryInterceptor())

	cID1 := ident.MustParseClinic("01990000-0000-7000-8000-000000000001")
	cID2 := ident.MustParseClinic("01990000-0000-7000-8000-000000000002")

	// 1. Non-clinic scoped query without clinic in context passes through
	_, err = client.Clinic.Query().All(context.Background())
	require.NoError(t, err)

	// 2. Non-clinic scoped mutation without clinic in context passes through
	_, err = client.Clinic.Create().SetID(cID1).SetSlug("c1").SetName("C1").Save(context.Background())
	require.NoError(t, err)

	// 3. Mutation on clinic-scoped entity without clinic in context fails with ErrMissingClinic
	_, err = client.Role.Create().SetName("custom").Save(context.Background())
	assert.ErrorIs(t, err, clinicctx.ErrMissingClinic)

	// 4. Create with explicit mismatched clinic_id fails
	ctx1 := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{ID: cID1, Slug: "c1", Name: "C1"})
	_, err = client.Role.Create().SetClinicID(cID2).SetName("custom").Save(ctx1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "isolation: clinic_id mismatch")

	// 5. Update with clinic context restricts to clinic
	r1, err := client.Role.Create().SetName("admin").SetSystem(true).Save(ctx1)
	require.NoError(t, err)

	ctx2 := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{ID: cID2, Slug: "c2", Name: "C2"})
	err = client.Role.UpdateOneID(r1.ID).SetName("renamed").Exec(ctx2)
	assert.Error(t, err) // Not found in clinic 2
}
