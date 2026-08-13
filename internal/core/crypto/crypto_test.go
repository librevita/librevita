package crypto

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"

	"librevita.org/internal/core/config"
)

const testKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA="

func mustKey(t *testing.T) *MasterKey {
	t.Helper()
	key, err := NewMasterKey(testKey)
	if err != nil {
		t.Fatalf("NewMasterKey: %v", err)
	}
	return key
}

func TestNewMasterKeyRejectsInvalidInput(t *testing.T) {
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
			if _, err := NewMasterKey(tc.encoded); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSealOpenRoundtrip(t *testing.T) {
	key := mustKey(t)
	aad := []byte("urn:librevita:id:br:cpf")

	ct, nonce, err := key.Seal(aad, []byte("12345678900"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if len(nonce) != 24 {
		t.Fatalf("nonce length = %d, want 24", len(nonce))
	}
	if bytes.Equal(ct, []byte("12345678900")) {
		t.Fatal("ciphertext equals plaintext")
	}

	got, err := key.Open(aad, ct, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != "12345678900" {
		t.Fatalf("plaintext = %q, want %q", got, "12345678900")
	}
}

func TestOpenRejectsTampering(t *testing.T) {
	key := mustKey(t)
	aad := []byte("urn:librevita:id:br:cpf")
	ct, nonce, err := key.Seal(aad, []byte("12345678900"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	cases := []struct {
		name    string
		ct      []byte
		nonce   []byte
		aad     []byte
		replica []byte
	}{
		{"ciphertext", ct, nonce, aad, []byte("12345678901")},
		{"nonce", ct, []byte("012345678901234567890123"), aad, nil},
		{"aad", ct, nonce, []byte("urn:librevita:id:br:sus"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ct2 := tc.ct
			if tc.replica != nil {
				ct2 = bytes.Clone(tc.ct)
				ct2[len(ct2)-1] ^= 0x01
			}
			if _, err := key.Open(tc.aad, ct2, tc.nonce); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestBlindIndexDeterministicAndSeparated(t *testing.T) {
	key := mustKey(t)
	index, err := key.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if len(index) != 64 {
		t.Fatalf("index length = %d, want 64", len(index))
	}
	if !isHex(index) {
		t.Fatalf("index is not hex: %q", index)
	}

	again, err := key.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if index != again {
		t.Fatalf("index is not deterministic: %q != %q", index, again)
	}

	// Domain separation: the system URN participates, so identical
	// values under different systems never collide.
	otherSystem, err := key.BlindIndex("urn:librevita:id:br:sus", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if otherSystem == index {
		t.Fatal("different systems produced the same index")
	}

	otherValue, err := key.BlindIndex("urn:librevita:id:br:cpf", "12345678901")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if otherValue == index {
		t.Fatal("different values produced the same index")
	}
}

func TestBlindIndexChangesWithKey(t *testing.T) {
	other, err := NewMasterKey("6pocGlmi1FiLlWEbUX1JW84vYTCrMRssMdnIwUkFbBc=")
	if err != nil {
		t.Fatalf("NewMasterKey: %v", err)
	}
	key := mustKey(t)

	index, err := key.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	otherIndex, err := other.BlindIndex("urn:librevita:id:br:cpf", "12345678900")
	if err != nil {
		t.Fatalf("BlindIndex: %v", err)
	}
	if index == otherIndex {
		t.Fatal("a different master key produced the same index")
	}
}

func TestNewFromConfigKeyPolicy(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	// Outside development the key is required.
	for _, env := range []string{"production", "staging"} {
		cfg := &config.Config{Mode: env}
		if _, err := NewFromConfig(cfg, log); err == nil {
			t.Fatalf("NewFromConfig(%s) without key = nil error, want error", env)
		}
	}

	// A configured key always works.
	cfg := &config.Config{Mode: "production", MasterKey: testKey}
	if _, err := NewFromConfig(cfg, log); err != nil {
		t.Fatalf("NewFromConfig with key: %v", err)
	}

	// Development falls back to an ephemeral key.
	dev := &config.Config{Mode: "development"}
	key, err := NewFromConfig(dev, log)
	if err != nil {
		t.Fatalf("NewFromConfig(development): %v", err)
	}
	ct, nonce, err := key.Seal(nil, []byte("value"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := key.Open(nil, ct, nonce); err != nil {
		t.Fatalf("Open: %v", err)
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
