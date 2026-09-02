package fle_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/enttest"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/keystore"
)

func generateTestKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	_, err := rand.Read(k)
	require.NoError(t, err)
	return k
}

func TestEncryptedValueScanner_Stateless(t *testing.T) {
	scanner := fle.EncryptedString("test")

	// Value returns string as-is
	v, err := scanner.Value("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", v)

	// ScanValue returns sql.NullString
	sv := scanner.ScanValue()
	require.IsType(t, &sql.NullString{}, sv)

	// FromValue parses null string
	res, err := scanner.FromValue(&sql.NullString{String: "cleartext", Valid: true})
	require.NoError(t, err)
	assert.Equal(t, "cleartext", res)

	// Nil / invalid returns empty string
	res, err = scanner.FromValue(&sql.NullString{Valid: false})
	require.NoError(t, err)
	assert.Equal(t, "", res)
}

func TestResolveEncryptor_ContextHierarchy(t *testing.T) {
	keyDef := generateTestKey(t)
	defEnc, err := crypto.NewPatientEncryptor(keyDef)
	require.NoError(t, err)

	keyCtx := generateTestKey(t)
	ctxEnc, err := crypto.NewPatientEncryptor(keyCtx)
	require.NoError(t, err)

	keyRes := generateTestKey(t)
	resEnc, err := crypto.NewPatientEncryptor(keyRes)
	require.NoError(t, err)

	// 1. Fallback to default when context is empty
	resolved, err := fle.ResolveEncryptor(context.Background(), defEnc)
	require.NoError(t, err)
	assert.Equal(t, defEnc, resolved)

	// 2. Direct WithEncryptor takes highest precedence
	ctxWithEnc := fle.WithEncryptor(context.Background(), ctxEnc)
	resolved, err = fle.ResolveEncryptor(ctxWithEnc, defEnc)
	require.NoError(t, err)
	assert.Equal(t, ctxEnc, resolved)

	// 3. EncryptorResolver takes precedence over default
	resolver := fle.EncryptorResolverFunc(func(ctx context.Context) (crypto.Encryptor, error) {
		return resEnc, nil
	})
	ctxWithRes := fle.WithEncryptorResolver(context.Background(), resolver)
	resolved, err = fle.ResolveEncryptor(ctxWithRes, defEnc)
	require.NoError(t, err)
	assert.Equal(t, resEnc, resolved)

	// 4. Direct WithEncryptor overrides EncryptorResolver
	ctxBoth := fle.WithEncryptor(ctxWithRes, ctxEnc)
	resolved, err = fle.ResolveEncryptor(ctxBoth, defEnc)
	require.NoError(t, err)
	assert.Equal(t, ctxEnc, resolved)
}

func setupTestEntClient(t *testing.T, hasher crypto.Hasher, defaultEnc crypto.Encryptor) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:ent_fle?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	client.Use(ent.FLEMutationHook(hasher, defaultEnc))
	client.Intercept(ent.FLEDecryptionInterceptor(defaultEnc))
	return client
}

func TestFLE_MultiTenant_DynamicKey_Concurrency(t *testing.T) {
	// Setup master hasher and client without default encryptor (force context resolution)
	masterKey := generateTestKey(t)
	hasher, err := crypto.NewMasterIndexHasher(masterKey)
	require.NoError(t, err)

	client := setupTestEntClient(t, hasher, nil)

	// Create 2 clinics (tenants)
	clinic1ID := ident.ClinicID(uuid.New())
	_, err = client.Clinic.Create().SetID(clinic1ID).SetSlug("tenant-a").SetName("Clínica Tenant A").Save(context.Background())
	require.NoError(t, err)

	clinic2ID := ident.ClinicID(uuid.New())
	_, err = client.Clinic.Create().SetID(clinic2ID).SetSlug("tenant-b").SetName("Clínica Tenant B").Save(context.Background())
	require.NoError(t, err)

	// Distinct keys for each tenant
	keyTenant1 := generateTestKey(t)
	encTenant1, err := crypto.NewPatientEncryptor(keyTenant1)
	require.NoError(t, err)

	keyTenant2 := generateTestKey(t)
	encTenant2, err := crypto.NewPatientEncryptor(keyTenant2)
	require.NoError(t, err)

	const iterations = 20
	var wg sync.WaitGroup

	// Worker 1: Tenant A
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ctx := fle.WithEncryptor(context.Background(), encTenant1)
			ctx = fle.WithClinicID(ctx, clinic1ID)

			pID := ident.PatientID(uuid.New())
			name := fmt.Sprintf("Patient Tenant A %d", i)
			p, err := client.Patient.Create().
				SetID(pID).
				SetClinicID(clinic1ID).
				SetDisplayName(name).
				SetPhone("+55 11 91111-1111").
				SetEmail(fmt.Sprintf("tenant_a_%d@example.com", i)).
				Save(ctx)
			assert.NoError(t, err)
			assert.Equal(t, name, p.DisplayName)

			// Fetch with tenant 1 context -> should decrypt properly
			fetched, err := client.Patient.Get(ctx, pID)
			assert.NoError(t, err)
			assert.Equal(t, name, fetched.DisplayName)

			// Fetch with wrong tenant context -> cannot decrypt, raw ciphertext returned
			wrongCtx := fle.WithEncryptor(context.Background(), encTenant2)
			wrongFetched, err := client.Patient.Get(wrongCtx, pID)
			assert.Error(t, err)
			assert.Nil(t, wrongFetched)
		}
	}()

	// Worker 2: Tenant B
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ctx := fle.WithEncryptor(context.Background(), encTenant2)
			ctx = fle.WithClinicID(ctx, clinic2ID)

			pID := ident.PatientID(uuid.New())
			name := fmt.Sprintf("Patient Tenant B %d", i)
			p, err := client.Patient.Create().
				SetID(pID).
				SetClinicID(clinic2ID).
				SetDisplayName(name).
				SetPhone("+55 21 92222-2222").
				SetEmail(fmt.Sprintf("tenant_b_%d@example.com", i)).
				Save(ctx)
			assert.NoError(t, err)
			assert.Equal(t, name, p.DisplayName)

			// Fetch with tenant 2 context -> should decrypt properly
			fetched, err := client.Patient.Get(ctx, pID)
			assert.NoError(t, err)
			assert.Equal(t, name, fetched.DisplayName)

			// Fetch with wrong tenant context -> cannot decrypt, raw ciphertext returned
			wrongCtx := fle.WithEncryptor(context.Background(), encTenant1)
			wrongFetched, err := client.Patient.Get(wrongCtx, pID)
			assert.Error(t, err)
			assert.Nil(t, wrongFetched)
		}
	}()

	wg.Wait()
}

func TestFLE_UsesPatientDEKPerEntity(t *testing.T) {
	db, err := sql.Open("sqlite", "file:patient_scoped_fle?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Schema.Create(context.Background()))
	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	master := generateTestKey(t)
	engine, err := crypto.NewEngine(base64.StdEncoding.EncodeToString(master), v)
	require.NoError(t, err)
	clinicID := ident.ClinicID(uuid.New())
	patientA := ident.PatientID(uuid.New())
	patientB := ident.PatientID(uuid.New())
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("patient-scoped").
		SetName("Patient Scoped").
		SetCountry("BR").
		SetTimezone("America/Sao_Paulo").
		Save(context.Background())
	require.NoError(t, err)
	clinicDEK, err := engine.EnsureClinicDEK(context.Background(), clinicID)
	require.NoError(t, err)
	clinicEnc, err := crypto.NewClinicEncryptor(clinicDEK)
	require.NoError(t, err)
	clinicHasher, err := crypto.NewHasherFromDEK(clinicDEK)
	require.NoError(t, err)
	crypto.ZeroBytes(clinicDEK)

	client.Use(ent.FLEMutationHook(clinicHasher, clinicEnc, engine))
	client.Intercept(ent.FLEDecryptionInterceptor(clinicEnc, engine))

	ctx := fle.WithClinicID(context.Background(), clinicID)
	ctx = crypto.WithRequestKeyCache(ctx)
	defer crypto.ClearRequestKeyCache(ctx)
	ctx = fle.WithEncryptor(ctx, clinicEnc)
	ctx = fle.WithHasher(ctx, clinicHasher)
	ctx = fle.WithPatientEncryptorResolver(ctx, engine)

	_, err = engine.EnsurePatientDEKForClinic(ctx, clinicID, patientA)
	require.NoError(t, err)
	_, err = engine.EnsurePatientDEKForClinic(ctx, clinicID, patientB)
	require.NoError(t, err)
	_, err = client.Patient.Create().
		SetID(patientA).
		SetClinicID(clinicID).
		SetDisplayName("Paciente A").
		SetPhone("5511999990001").
		SetEmail("a@example.org").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.Patient.Create().
		SetID(patientB).
		SetClinicID(clinicID).
		SetDisplayName("Paciente B").
		SetPhone("5511999990002").
		SetEmail("b@example.org").
		Save(ctx)
	require.NoError(t, err)

	var rawA, rawB []byte
	require.NoError(t, db.QueryRowContext(ctx, "SELECT display_name FROM patients WHERE id = ?", patientA).Scan(&rawA))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT display_name FROM patients WHERE id = ?", patientB).Scan(&rawB))
	assert.NotEqual(t, "Paciente A", string(rawA))
	assert.NotEqual(t, "Paciente B", string(rawB))
	assert.NotEqual(t, rawA, rawB)

	gotA, err := client.Patient.Get(ctx, patientA)
	require.NoError(t, err)
	assert.Equal(t, "Paciente A", gotA.DisplayName)
	require.NoError(t, engine.DeletePatientDEKForClinic(ctx, clinicID, patientA))
	_, err = client.Patient.Get(ctx, patientA)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)

	gotB, err := client.Patient.Get(ctx, patientB)
	require.NoError(t, err)
	assert.Equal(t, "Paciente B", gotB.DisplayName)
}
