package crypto_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/vault"
)

const testKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA="

func mustVault(t *testing.T) crypto.KeyVault {
	t.Helper()
	v, err := vault.NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	return v
}

func mustEngine(t *testing.T) *crypto.Engine {
	t.Helper()
	v := mustVault(t)
	eng, err := crypto.NewEngine(testKey, v)
	require.NoError(t, err)
	return eng
}

func TestNewEngineRejectsInvalidInput(t *testing.T) {
	v := mustVault(t)
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

func TestNewEngineRequiresVault(t *testing.T) {
	_, err := crypto.NewEngine(testKey, nil)
	assert.Error(t, err)
}

func TestKEKDEKPatientDataEncryptionAndCryptoShredding(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	v, err := vault.NewBBoltVault(filepath.Join(dir, "keys.db"))
	require.NoError(t, err)
	defer func() { _ = v.Close() }()

	eng, err := crypto.NewEngine(testKey, v)
	require.NoError(t, err)

	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000a1")
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000099")
	patientURN := crypto.PatientURN(clinicID, patientID)
	aad := []byte(patientURN)
	plaintext := []byte("12345678900")

	// 1. Setup Patient DEK
	dek, err := eng.SetupPatientDEK(ctx, patientURN)
	require.NoError(t, err)
	assert.Len(t, dek, 32)

	// Verify DEK is stored encrypted in vault and can be retrieved
	retrievedDEK, err := eng.GetPatientDEK(ctx, patientURN)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(dek, retrievedDEK))

	// 2. Encrypt Patient Data using DEK
	ct, nonce, err := eng.EncryptPatientData(ctx, patientURN, aad, plaintext)
	require.NoError(t, err)
	assert.Len(t, nonce, 24)

	// 3. Decrypt Patient Data using DEK
	got, err := eng.DecryptPatientData(ctx, patientURN, aad, ct, nonce)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	// 4. Crypto-Shredding (Delete Patient DEK)
	err = eng.DeletePatientDEK(ctx, patientURN)
	require.NoError(t, err)

	// 5. Decryption must fail after Crypto-Shredding with a terminal error.
	_, err = eng.DecryptPatientData(ctx, patientURN, aad, ct, nonce)
	assert.ErrorIs(t, err, crypto.ErrKeyDestroyed)
}

func TestSealOpenDirectKEKRoundtrip(t *testing.T) {
	eng := mustEngine(t)
	aad := []byte("urn:librevita:system:config")

	ct, nonce, err := eng.Seal(aad, []byte("secret-payload"))
	require.NoError(t, err)

	got, err := eng.Open(aad, ct, nonce)
	require.NoError(t, err)
	assert.Equal(t, "secret-payload", string(got))
}

func TestBlindIndexDeterministicAndSeparated(t *testing.T) {
	eng := mustEngine(t)
	index, err := eng.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	require.NoError(t, err)
	assert.Contains(t, index, "$")
	parts := strings.Split(index, "$")
	require.Len(t, parts, 2)
	assert.Equal(t, "blake2s", parts[0])
	assert.Len(t, parts[1], 64)
	assert.True(t, isHex(parts[1]))

	again, err := eng.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	require.NoError(t, err)
	assert.Equal(t, index, again)

	otherSystem, err := eng.BlindIndex("urn:librevita:id:br:sus", "12345678900")
	require.NoError(t, err)
	assert.NotEqual(t, index, otherSystem)
}

func TestNewFromConfigKeyPolicy(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	v := mustVault(t)

	for _, env := range []string{"production", "staging"} {
		cfg := &config.Config{Mode: env}
		_, err := crypto.NewFromConfig(cfg, v, log)
		assert.Error(t, err, "NewFromConfig(%s) without key should fail", env)
	}

	cfg := &config.Config{Mode: "production", MasterKey: testKey}
	_, err := crypto.NewFromConfig(cfg, v, log)
	require.NoError(t, err)

	dev := &config.Config{Mode: "development"}
	eng, err := crypto.NewFromConfig(dev, v, log)
	require.NoError(t, err)

	ct, nonce, err := eng.Seal(nil, []byte("value"))
	require.NoError(t, err)
	got, err := eng.Open(nil, ct, nonce)
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

	var keyVault crypto.KeyVault
	var eng *crypto.Engine
	var hasher crypto.Hasher
	var encryptor crypto.Encryptor

	app := fxtest.New(t,
		fx.Provide(
			func() *config.Config { return cfg },
			func() *slog.Logger { return slog.New(slog.DiscardHandler) },
		),
		vault.Module,
		crypto.Module,
		fx.Populate(&keyVault, &eng, &hasher, &encryptor),
	)
	app.RequireStart()
	defer app.RequireStop()

	require.NotNil(t, keyVault)
	require.NotNil(t, eng)
	require.NotNil(t, hasher)
	require.NotNil(t, encryptor)

	// Verify hasher
	h, err := hasher.HashString("test-session")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "blake2s$"))

	// Verify encryptor
	ct, err := encryptor.Encrypt([]byte("medical-data"), []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, crypto.MagicByteXChaCha20Poly1305, ct[0])
	pt, err := encryptor.Decrypt(ct, []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, []byte("medical-data"), pt)

	ctx := context.Background()
	pURN := crypto.PatientURN(
		uuid.MustParse("01990000-0000-7000-8000-0000000000a1"),
		uuid.MustParse("01990000-0000-7000-8000-0000000000ff"),
	)
	_, err = eng.SetupPatientDEK(ctx, pURN)
	require.NoError(t, err)
}

func TestClinicDEKEnvelopeAndCryptoShred(t *testing.T) {
	ctx := context.Background()
	eng := mustEngine(t)

	clinicA := uuid.MustParse("01990000-0000-7000-8000-0000000000a1")
	clinicB := uuid.MustParse("01990000-0000-7000-8000-0000000000a2")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000b1")

	dekA, err := eng.EnsureClinicDEK(ctx, clinicA)
	require.NoError(t, err)
	dekB, err := eng.EnsureClinicDEK(ctx, clinicB)
	require.NoError(t, err)
	assert.NotEqual(t, dekA, dekB)

	hA, err := crypto.NewHasherFromDEK(dekA)
	require.NoError(t, err)
	hB, err := crypto.NewHasherFromDEK(dekB)
	require.NoError(t, err)
	idxA, err := hA.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	require.NoError(t, err)
	idxB, err := hB.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	require.NoError(t, err)
	assert.NotEqual(t, idxA, idxB, "same catalog URN must not share a blind index across clinics")

	pURN := crypto.PatientURN(clinicA, patientID)
	aad := []byte(pURN)
	plaintext := []byte("PHI-norte")

	ct, nonce, err := eng.EncryptPatientData(ctx, pURN, aad, plaintext)
	require.NoError(t, err)
	got, err := eng.DecryptPatientData(ctx, pURN, aad, ct, nonce)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	_, err = eng.GetPatientDEKForClinic(ctx, clinicB, patientID)
	assert.Error(t, err, "clinic B must not hold clinic A's patient DEK")

	require.NoError(t, eng.DeleteClinicDEK(ctx, clinicA))
	_, err = eng.DecryptPatientData(ctx, pURN, aad, ct, nonce)
	assert.Error(t, err)
}

func TestBatchPatientDEKResolutionUsesOneVaultBatch(t *testing.T) {
	ctx := crypto.WithRequestKeyCache(context.Background())
	defer crypto.ClearRequestKeyCache(ctx)
	eng := mustEngine(t)
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000a1")
	patientA := uuid.MustParse("01990000-0000-7000-8000-0000000000b1")
	patientB := uuid.MustParse("01990000-0000-7000-8000-0000000000b2")

	_, err := eng.EnsurePatientDEKForClinic(ctx, clinicID, patientA)
	require.NoError(t, err)
	_, err = eng.EnsurePatientDEKForClinic(ctx, clinicID, patientB)
	require.NoError(t, err)
	before := eng.KeyMetrics()
	crypto.ClearRequestKeyCache(ctx)

	deks, err := eng.GetPatientDEKsForClinic(ctx, clinicID, []uuid.UUID{patientA, patientB, patientA})
	require.NoError(t, err)
	require.Len(t, deks, 2)
	assert.Len(t, deks[patientA], crypto.SizeDEK)
	assert.Len(t, deks[patientB], crypto.SizeDEK)
	metrics := eng.KeyMetrics()
	assert.Equal(t, uint64(1), metrics.VaultBatchGet-before.VaultBatchGet)
	assert.Equal(t, uint64(1), metrics.VaultGet-before.VaultGet)
}

func TestConcurrentClinicDEKProvisioningIsCreateIfAbsent(t *testing.T) {
	eng := mustEngine(t)
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000a3")
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
