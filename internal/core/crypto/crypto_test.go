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

	patientURN := "urn:librevita:patient:018f1234-5678-7000-8000-000000000099"
	aad := []byte("urn:librevita:id:br:cpf")
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

	// 5. Decryption must fail after Crypto-Shredding with ErrKeyNotFound
	_, err = eng.DecryptPatientData(ctx, patientURN, aad, ct, nonce)
	assert.ErrorIs(t, err, crypto.ErrKeyNotFound)
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
	assert.Len(t, index, 64)
	assert.True(t, isHex(index))

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
	pURN := "urn:librevita:patient:fx-test"
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

func TestReenvelopePatientDEKFromLegacyKEK(t *testing.T) {
	ctx := context.Background()
	eng := mustEngine(t)
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000a1")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000b2")

	legacyURN := crypto.LegacyPatientURN(patientID)
	legacyDEK, err := eng.SetupPatientDEK(ctx, legacyURN)
	require.NoError(t, err)

	require.NoError(t, eng.ReenvelopePatientDEK(ctx, clinicID, patientID))
	got, err := eng.GetPatientDEKForClinic(ctx, clinicID, patientID)
	require.NoError(t, err)
	assert.Equal(t, legacyDEK, got)
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
