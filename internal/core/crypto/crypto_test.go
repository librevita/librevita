package crypto_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/keystore"
	"librevita.org/pkg/log"
	"librevita.org/pkg/urn"
)

const testKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=" // gitleaks:allow

func mustKeyStore(t *testing.T) crypto.KeyStore {
	t.Helper()
	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	return v
}

func mustEngine(t *testing.T) *crypto.Engine {
	t.Helper()
	v := mustKeyStore(t)
	eng, err := crypto.NewEngine(testKey, v)
	require.NoError(t, err)
	return eng
}

func TestNewEngineRejectsInvalidInput(t *testing.T) {
	v := mustKeyStore(t)
	cases := []struct {
		name    string
		encoded string
	}{
		{"empty", ""},
		{"not base64", "!!!not-base64!!!"},
		{"wrong size", base64.StdEncoding.EncodeToString([]byte("short"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := crypto.NewEngine(tc.encoded, v)
			assert.Error(t, err)
		})
	}
}

func TestNewEngineRequiresKeyStore(t *testing.T) {
	_, err := crypto.NewEngine(testKey, nil)
	assert.Error(t, err)
}

func TestKEKDEKPatientDataEncryptionAndCryptoShredding(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	v, err := keystore.OpenBBolt(filepath.Join(dir, "keystore.db"))
	require.NoError(t, err)
	defer func() { _ = v.Close() }()

	eng, err := crypto.NewEngine(testKey, v)
	require.NoError(t, err)

	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-000000000099")
	patientURN := urn.Patient(clinicID, patientID)
	aad := []byte(patientURN)
	plaintext := []byte("12345678900")

	// 1. Setup Patient DEK
	dek, err := eng.SetupPatientDEK(ctx, patientURN)
	require.NoError(t, err)
	assert.Len(t, dek, 32)

	// Verify DEK is stored encrypted in the keystore and can be retrieved
	retrievedDEK, err := eng.GetPatientDEK(ctx, patientURN)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(dek, retrievedDEK))

	// 2. Encrypt Patient Data using DEK
	ct, err := eng.EncryptPatientData(ctx, patientURN, aad, plaintext)
	require.NoError(t, err)
	assert.Equal(t, crypto.MagicByteXChaCha20Poly1305, ct[0])
	assert.Equal(t, crypto.KeyScopePatient, ct[1])
	assert.Equal(t, crypto.DefaultKeyID, ct[2])

	// 3. Decrypt Patient Data using DEK
	got, err := eng.DecryptPatientData(ctx, patientURN, aad, ct)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	// 4. Crypto-Shredding (Delete Patient DEK)
	err = eng.DeletePatientDEK(ctx, patientURN)
	require.NoError(t, err)

	// 5. Decryption must fail after Crypto-Shredding with a terminal error.
	_, err = eng.DecryptPatientData(ctx, patientURN, aad, ct)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
}

func TestSealOpenDirectKEKRoundtrip(t *testing.T) {
	eng := mustEngine(t)
	aad := []byte("urn:librevita:system:config")

	ct, err := eng.Seal(aad, []byte("secret-payload"))
	require.NoError(t, err)
	assert.Equal(t, crypto.KeyScopeMaster, ct[1])
	assert.Equal(t, crypto.DefaultKeyID, ct[2])

	got, err := eng.Open(aad, ct)
	require.NoError(t, err)
	assert.Equal(t, "secret-payload", string(got))
}

func TestBlindIndexDeterministicAndSeparated(t *testing.T) {
	eng := mustEngine(t)
	index, err := eng.BlindIndex(urn.Identifier("br", "cpf"), "12345678900")
	require.NoError(t, err)
	assert.Contains(t, index, "$")
	parts := strings.Split(index, "$")
	require.Len(t, parts, 4)
	assert.Equal(t, "blake2s", parts[0])
	assert.Equal(t, "mi", parts[1])
	assert.Equal(t, "01", parts[2])
	assert.Len(t, parts[3], 64)
	assert.True(t, isHex(parts[3]))

	again, err := eng.BlindIndex(urn.Identifier("br", "cpf"), "12345678900")
	require.NoError(t, err)
	assert.Equal(t, index, again)

	otherSystem, err := eng.BlindIndex(urn.Identifier("br", "sus"), "12345678900")
	require.NoError(t, err)
	assert.NotEqual(t, index, otherSystem)
}

func TestNewFromConfigKeyPolicy(t *testing.T) {
	logger := log.Nop()
	v := mustKeyStore(t)

	for _, env := range []string{"production", "staging"} {
		cfg := &config.Config{Mode: env}
		_, err := crypto.NewFromConfig(cfg, v, logger)
		assert.Error(t, err, "NewFromConfig(%s) without key should fail", env)
	}

	cfg := &config.Config{Mode: "production", MasterKey: testKey}
	_, err := crypto.NewFromConfig(cfg, v, logger)
	require.NoError(t, err)

	dev := &config.Config{Mode: "development"}
	eng, err := crypto.NewFromConfig(dev, v, logger)
	require.NoError(t, err)

	ct, err := eng.Seal(nil, []byte("value"))
	require.NoError(t, err)
	got, err := eng.Open(nil, ct)
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), got)
}

func TestFxModuleIntegration(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Mode:      "development",
		DataDir:   dir,
		MasterKey: testKey,
	}

	var ks crypto.KeyStore
	var eng *crypto.Engine
	var hasher crypto.Hasher
	var encryptor crypto.Encryptor

	app := fxtest.New(t,
		fx.Provide(
			func() *config.Config { return cfg },
			func() log.Logger { return log.Nop() },
		),
		keystore.Module,
		crypto.Module,
		fx.Populate(&ks, &eng, &hasher, &encryptor),
	)
	app.RequireStart()
	defer app.RequireStop()

	require.NotNil(t, ks)
	require.NotNil(t, eng)
	require.NotNil(t, hasher)
	require.NotNil(t, encryptor)

	// Verify hasher
	h, err := hasher.HashString("test-session")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "blake2s$mi$01$"))

	// Verify encryptor
	ct, err := encryptor.Encrypt([]byte("medical-data"), []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, crypto.MagicByteXChaCha20Poly1305, ct[0])
	pt, err := encryptor.Decrypt(ct, []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, []byte("medical-data"), pt)

	ctx := context.Background()
	pURN := urn.Patient(
		ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1"),
		ident.MustParsePatient("01990000-0000-7000-8000-0000000000ff"),
	)
	_, err = eng.SetupPatientDEK(ctx, pURN)
	require.NoError(t, err)
}

func TestClinicDEKEnvelopeAndCryptoShred(t *testing.T) {
	ctx := context.Background()
	eng := mustEngine(t)

	clinicA := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")
	clinicB := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a2")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000b1")

	dekA, err := eng.EnsureClinicDEK(ctx, clinicA)
	require.NoError(t, err)
	dekB, err := eng.EnsureClinicDEK(ctx, clinicB)
	require.NoError(t, err)
	assert.NotEqual(t, dekA, dekB)

	hA, err := crypto.NewHasherFromDEK(dekA)
	require.NoError(t, err)
	assert.Equal(t, crypto.KeyScopeClinic, hA.KeyScope())
	assert.Equal(t, crypto.KeyPurposeIndex, hA.KeyPurpose())
	hB, err := crypto.NewHasherFromDEK(dekB)
	require.NoError(t, err)
	idxA, err := hA.BlindIndex(urn.Identifier("br", "cpf"), "12345678900")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(idxA, "blake2s$ci$01$"))
	idxB, err := hB.BlindIndex(urn.Identifier("br", "cpf"), "12345678900")
	require.NoError(t, err)
	assert.NotEqual(t, idxA, idxB, "same catalog URN must not share a blind index across clinics")

	pURN := urn.Patient(clinicA, patientID)
	aad := []byte(pURN)
	plaintext := []byte("PHI-norte")

	ct, err := eng.EncryptPatientData(ctx, pURN, aad, plaintext)
	require.NoError(t, err)
	got, err := eng.DecryptPatientData(ctx, pURN, aad, ct)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	_, err = eng.GetPatientDEKForClinic(ctx, clinicB, patientID)
	assert.Error(t, err, "clinic B must not hold clinic A's patient DEK")

	require.NoError(t, eng.DeleteClinicDEK(ctx, clinicA))
	_, err = eng.DecryptPatientData(ctx, pURN, aad, ct)
	assert.Error(t, err)
}

func TestClinicDEKByURN(t *testing.T) {
	ctx := context.Background()
	eng := mustEngine(t)
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000c1")
	clinicURN := urn.Clinic(clinicID)
	aad := []byte(clinicURN)

	dek, err := eng.SetupClinicDEK(ctx, clinicURN)
	require.NoError(t, err)
	assert.Len(t, dek, crypto.SizeDEK)

	got, err := eng.GetClinicDEKForURN(ctx, clinicURN)
	require.NoError(t, err)
	assert.Equal(t, dek, got)

	ct, err := eng.EncryptPayload(ctx, clinicURN, aad, []byte("clinic-owned"))
	require.NoError(t, err)
	assert.Equal(t, crypto.KeyScopeClinic, ct[1])
	assert.Equal(t, crypto.DefaultKeyID, ct[2])
	plain, err := eng.DecryptPayload(ctx, clinicURN, aad, ct)
	require.NoError(t, err)
	assert.Equal(t, []byte("clinic-owned"), plain)

	_, err = eng.SetupClinicDEK(ctx, "not-a-clinic-urn")
	assert.Error(t, err)
	_, err = eng.EncryptPayload(ctx, urn.PlatformSession("blake2s$abc"), aad, []byte("x"))
	assert.Error(t, err)

	require.NoError(t, eng.DeleteClinicDEKForURN(ctx, clinicURN))
	_, err = eng.DecryptPayload(ctx, clinicURN, aad, ct)
	assert.Error(t, err)
}

func TestBatchPatientDEKResolutionUsesOneKeyStoreBatch(t *testing.T) {
	ctx := crypto.WithRequestKeyCache(context.Background())
	defer crypto.ClearRequestKeyCache(ctx)
	eng := mustEngine(t)
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")
	patientA := ident.MustParsePatient("01990000-0000-7000-8000-0000000000b1")
	patientB := ident.MustParsePatient("01990000-0000-7000-8000-0000000000b2")

	_, err := eng.EnsurePatientDEKForClinic(ctx, clinicID, patientA)
	require.NoError(t, err)
	_, err = eng.EnsurePatientDEKForClinic(ctx, clinicID, patientB)
	require.NoError(t, err)
	before := eng.KeyMetrics()
	crypto.ClearRequestKeyCache(ctx)

	deks, err := eng.GetPatientDEKsForClinic(ctx, clinicID, []ident.PatientID{patientA, patientB, patientA})
	require.NoError(t, err)
	require.Len(t, deks, 2)
	assert.Len(t, deks[patientA], crypto.SizeDEK)
	assert.Len(t, deks[patientB], crypto.SizeDEK)
	metrics := eng.KeyMetrics()
	assert.Equal(t, uint64(1), metrics.KeyStoreBatchGet-before.KeyStoreBatchGet)
	assert.Equal(t, uint64(1), metrics.KeyStoreGet-before.KeyStoreGet)
}

func TestConcurrentClinicDEKProvisioningIsCreateIfAbsent(t *testing.T) {
	eng := mustEngine(t)
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a3")
	const workers = 16
	results := make(chan []byte, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ctx := crypto.WithRequestKeyCache(context.Background())
			defer crypto.ClearRequestKeyCache(ctx)
			dek, err := eng.EnsureClinicDEK(ctx, clinicID)
			if err != nil {
				errs <- err
				return
			}
			results <- append([]byte(nil), dek...)
			crypto.ZeroBytes(dek)
		}()
	}
	all := make([][]byte, 0, workers)
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			require.NoError(t, err)
		case dek := <-results:
			require.Len(t, dek, crypto.SizeDEK)
			all = append(all, dek)
		}
	}
	require.Len(t, all, workers)
	first := all[0]
	for _, next := range all[1:] {
		assert.Equal(t, first, next)
		crypto.ZeroBytes(next)
	}
	crypto.ZeroBytes(first)
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
