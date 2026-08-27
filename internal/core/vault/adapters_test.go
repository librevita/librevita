package vault

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
)

func TestAdapterConstructorsValidation(t *testing.T) {
	_, err := NewNATSVault("", "bucket")
	assert.Error(t, err, "expected error for empty NATS URL")

	_, err = NewEtcdVault("", "/prefix")
	assert.Error(t, err, "expected error for empty etcd endpoints")

	_, err = NewHashiCorpVault("", "token", "mount")
	assert.Error(t, err, "expected error for empty HashiCorp address")

	_, err = NewHashiCorpVault("http://localhost:8200", "", "mount")
	assert.Error(t, err, "expected error for empty HashiCorp token")
}

func TestSanitizeNATSKey(t *testing.T) {
	urn := "urn:librevita:patient:123/456.789"
	assert.Equal(t, "k_"+base64.RawURLEncoding.EncodeToString([]byte(urn)), sanitizeNATSKey(urn))
}

func TestIsVaultNotFound(t *testing.T) {
	respErr := &api.ResponseError{StatusCode: http.StatusNotFound}
	assert.True(t, isVaultNotFound(respErr), "expected response error 404 to be recognized as not found")

	rawErr := errors.New("URL GET http://localhost:8200: 404 secret not found")
	assert.True(t, isVaultNotFound(rawErr), "expected raw 404 error to be recognized as not found")

	otherErr := errors.New("500 internal server error")
	assert.False(t, isVaultNotFound(otherErr), "expected 500 error not to be recognized as not found")
}
