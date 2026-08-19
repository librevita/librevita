package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"librevita.org/internal/core/config"
)

const testKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA="

func mustVault(t *testing.T) KeyVault {
	t.Helper()
	vault, err := NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatalf("NewBBoltVault: %v", err)
	}
	t.Cleanup(func() { _ = vault.Close() })
	return vault
}

func mustEngine(t *testing.T) *Engine {
	t.Helper()
	vault := mustVault(t)
	eng, err := NewEngine(testKey, vault)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

func TestNewEngineRejectsInvalidInput(t *testing.T) {
	vault := mustVault(t)
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
			if _, err := NewEngine(tc.encoded, vault); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNewEngineRequiresVault(t *testing.T) {
	if _, err := NewEngine(testKey, nil); err == nil {
		t.Fatal("expected error when vault is nil")
	}
}

func TestKEKDEKPatientDataEncryptionAndCryptoShredding(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	vault, err := NewBBoltVault(filepath.Join(dir, "keys.db"))
	if err != nil {
		t.Fatalf("NewBBoltVault: %v", err)
	}
	defer func() { _ = vault.Close() }()

	eng, err := NewEngine(testKey, vault)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	patientURN := "urn:librevita:patient:018f1234-5678-7000-8000-000000000099"
	aad := []byte("urn:librevita:id:br:cpf")
	plaintext := []byte("12345678900")

	// 1. Setup Patient DEK
	dek, err := eng.SetupPatientDEK(ctx, patientURN)
	if err != nil {
		t.Fatalf("SetupPatientDEK: %v", err)
	}
	if len(dek) != 32 {
		t.Fatalf("DEK length = %d, want 32", len(dek))
	}

	// Verify DEK is stored encrypted in vault and can be retrieved
	retrievedDEK, err := eng.GetPatientDEK(ctx, patientURN)
	if err != nil {
		t.Fatalf("GetPatientDEK: %v", err)
	}
	if !bytes.Equal(dek, retrievedDEK) {
		t.Fatal("retrieved DEK does not match setup DEK")
	}

	// 2. Encrypt Patient Data using DEK
	ct, nonce, err := eng.EncryptPatientData(ctx, patientURN, aad, plaintext)
	if err != nil {
		t.Fatalf("EncryptPatientData: %v", err)
	}
	if len(nonce) != 24 {
		t.Fatalf("nonce length = %d, want 24", len(nonce))
	}

	// 3. Decrypt Patient Data using DEK
	got, err := eng.DecryptPatientData(ctx, patientURN, aad, ct, nonce)
	if err != nil {
		t.Fatalf("DecryptPatientData: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypted = %q, want %q", got, plaintext)
	}

	// 4. Crypto-Shredding (Delete Patient DEK)
	if err := eng.DeletePatientDEK(ctx, patientURN); err != nil {
		t.Fatalf("DeletePatientDEK: %v", err)
	}

	// 5. Decryption must fail after Crypto-Shredding with ErrKeyNotFound
	_, err = eng.DecryptPatientData(ctx, patientURN, aad, ct, nonce)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("DecryptPatientData after shredding = %v, want %v", err, ErrKeyNotFound)
	}
}

func TestBBoltVaultDirectOperations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "keys.db")

	vault, err := NewBBoltVault(dbPath)
	if err != nil {
		t.Fatalf("NewBBoltVault: %v", err)
	}
	defer func() { _ = vault.Close() }()

	ctx := context.Background()
	patientURN := "urn:librevita:patient:018f7654-3210-7000-8000-000000000001"
	encDEK := []byte("encrypted-dek-payload-32-bytes!!")

	// Get before Put should return ErrKeyNotFound
	if _, err := vault.GetDEK(ctx, patientURN); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("GetDEK before Put = %v, want %v", err, ErrKeyNotFound)
	}

	// Put DEK
	if err := vault.PutDEK(ctx, patientURN, encDEK); err != nil {
		t.Fatalf("PutDEK: %v", err)
	}

	// Get DEK
	got, err := vault.GetDEK(ctx, patientURN)
	if err != nil {
		t.Fatalf("GetDEK: %v", err)
	}
	if !bytes.Equal(got, encDEK) {
		t.Fatalf("GetDEK = %q, want %q", got, encDEK)
	}

	// Delete DEK (Crypto-Shredding)
	if err := vault.DeleteDEK(ctx, patientURN); err != nil {
		t.Fatalf("DeleteDEK: %v", err)
	}

	// Get after Delete should return ErrKeyNotFound
	if _, err := vault.GetDEK(ctx, patientURN); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("GetDEK after Delete = %v, want %v", err, ErrKeyNotFound)
	}
}

func TestSealOpenDirectKEKRoundtrip(t *testing.T) {
	eng := mustEngine(t)
	aad := []byte("urn:librevita:system:config")

	ct, nonce, err := eng.Seal(aad, []byte("secret-payload"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	got, err := eng.Open(aad, ct, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != "secret-payload" {
		t.Fatalf("plaintext = %q, want %q", got, "secret-payload")
	}
}

func TestBlindIndexDeterministicAndSeparated(t *testing.T) {
	eng := mustEngine(t)
	index, err := eng.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if len(index) != 64 {
		t.Fatalf("index length = %d, want 64", len(index))
	}
	if !isHex(index) {
		t.Fatalf("index is not hex: %q", index)
	}

	again, err := eng.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if index != again {
		t.Fatalf("index is not deterministic: %q != %q", index, again)
	}

	otherSystem, err := eng.BlindIndex("urn:librevita:id:br:sus", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if otherSystem == index {
		t.Fatal("different systems produced the same index")
	}
}

func TestNewFromConfigKeyPolicy(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	vault := mustVault(t)

	for _, env := range []string{"production", "staging"} {
		cfg := &config.Config{Mode: env}
		if _, err := NewFromConfig(cfg, vault, log); err == nil {
			t.Fatalf("NewFromConfig(%s) without key = nil error, want error", env)
		}
	}

	cfg := &config.Config{Mode: "production", MasterKey: testKey}
	if _, err := NewFromConfig(cfg, vault, log); err != nil {
		t.Fatalf("NewFromConfig with key: %v", err)
	}

	dev := &config.Config{Mode: "development"}
	eng, err := NewFromConfig(dev, vault, log)
	if err != nil {
		t.Fatalf("NewFromConfig(development): %v", err)
	}
	ct, nonce, err := eng.Seal(nil, []byte("value"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := eng.Open(nil, ct, nonce); err != nil {
		t.Fatalf("Open: %v", err)
	}
}

func TestFxModuleIntegration(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Mode:      "development",
		DataDir:   dir,
		MasterKey: testKey,
	}

	var vault KeyVault
	var eng *Engine

	app := fxtest.New(t,
		fx.Provide(
			func() *config.Config { return cfg },
			func() *slog.Logger { return slog.New(slog.DiscardHandler) },
		),
		Module,
		fx.Populate(&vault, &eng),
	)
	app.RequireStart()
	defer app.RequireStop()

	if vault == nil || eng == nil {
		t.Fatal("Fx population failed")
	}

	ctx := context.Background()
	pURN := "urn:librevita:patient:fx-test"
	if _, err := eng.SetupPatientDEK(ctx, pURN); err != nil {
		t.Fatalf("SetupPatientDEK in Fx module test: %v", err)
	}
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
