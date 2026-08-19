package vault

import (
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/vault/api"
)

func TestAdapterConstructorsValidation(t *testing.T) {
	if _, err := NewNATSVault("", "bucket"); err == nil {
		t.Fatal("expected error for empty NATS URL")
	}

	if _, err := NewEtcdVault("", "/prefix"); err == nil {
		t.Fatal("expected error for empty etcd endpoints")
	}

	if _, err := NewHashiCorpVault("", "token", "mount"); err == nil {
		t.Fatal("expected error for empty HashiCorp address")
	}

	if _, err := NewHashiCorpVault("http://localhost:8200", "", "mount"); err == nil {
		t.Fatal("expected error for empty HashiCorp token")
	}
}

func TestSanitizeNATSKey(t *testing.T) {
	urn := "urn:librevita:patient:123/456.789"
	got := sanitizeNATSKey(urn)
	want := "urn_librevita_patient_123_456_789"
	if got != want {
		t.Fatalf("sanitizeNATSKey = %q, want %q", got, want)
	}
}

func TestIsVaultNotFound(t *testing.T) {
	respErr := &api.ResponseError{StatusCode: http.StatusNotFound}
	if !isVaultNotFound(respErr) {
		t.Fatal("expected response error 404 to be recognized as not found")
	}

	rawErr := errors.New("URL GET http://localhost:8200: 404 secret not found")
	if !isVaultNotFound(rawErr) {
		t.Fatal("expected raw 404 error to be recognized as not found")
	}

	otherErr := errors.New("500 internal server error")
	if isVaultNotFound(otherErr) {
		t.Fatal("expected 500 error not to be recognized as not found")
	}
}
