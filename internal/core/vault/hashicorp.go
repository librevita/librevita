package vault

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/vault/api"

	"librevita.org/internal/core/crypto"
)

// HashiCorpVault implements crypto.KeyVault for HashiCorp Vault / OpenBao using KV v2 engine.
type HashiCorpVault struct {
	client *api.Client
	mount  string
}

// NewHashiCorpVault initializes a HashiCorp Vault / OpenBao API client.
func NewHashiCorpVault(address, token, mount string) (*HashiCorpVault, error) {
	if address == "" {
		return nil, errors.New("vault: hashicorp vault address is required")
	}
	if token == "" {
		return nil, errors.New("vault: hashicorp vault token is required")
	}
	if mount == "" {
		mount = "secret"
	}

	config := api.DefaultConfig()
	config.Address = address

	client, err := api.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "vault: hashicorp client init")
	}
	client.SetToken(token)

	return &HashiCorpVault{
		client: client,
		mount:  mount,
	}, nil
}

func (v *HashiCorpVault) secretPath(patientURN string) string {
	sanitized := base64.RawURLEncoding.EncodeToString([]byte(patientURN))
	return "librevita/deks/" + sanitized
}

// PutDEK stores the base64-encoded encrypted DEK in Vault KV v2 under secretPath.
func (v *HashiCorpVault) PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error {
	path := v.secretPath(patientURN)
	if _, err := v.GetDEK(ctx, patientURN); err != nil && !errors.Is(err, crypto.ErrKeyNotFound) {
		return err
	}
	data := map[string]interface{}{
		"dek": base64.StdEncoding.EncodeToString(encryptedDEK),
	}
	if _, err := v.client.KVv2(v.mount).Put(ctx, path, data); err != nil {
		return errors.Wrap(err, "vault: hashicorp put")
	}
	return nil
}

// GetDEK retrieves and decodes the encrypted DEK from Vault KV v2.
// Returns crypto.ErrKeyNotFound if the secret does not exist (HTTP 404).
func (v *HashiCorpVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	path := v.secretPath(patientURN)
	secret, err := v.client.KVv2(v.mount).Get(ctx, path)
	if err != nil {
		if isVaultNotFound(err) {
			return nil, crypto.ErrKeyNotFound
		}
		return nil, errors.Wrap(err, "vault: hashicorp get")
	}
	if secret == nil || secret.Data == nil {
		return nil, crypto.ErrKeyNotFound
	}
	rawDEK, ok := secret.Data["dek"].(string)
	if !ok || rawDEK == "" {
		return nil, crypto.ErrInvalidDEK
	}
	decoded, err := base64.StdEncoding.DecodeString(rawDEK)
	if err != nil {
		return nil, errors.Wrap(err, "vault: hashicorp decode dek")
	}
	if crypto.IsDestroyedDEK(decoded) {
		return nil, crypto.ErrKeyDestroyed
	}
	return decoded, nil
}

// GetDEKs retrieves multiple wrapped DEKs with bounded concurrent KV v2
// requests. KV v2 does not expose a native multi-read operation.
func (v *HashiCorpVault) GetDEKs(ctx context.Context, patientURNs []string) (map[string]crypto.DEKResult, error) {
	return batchGetWithWorkers(ctx, patientURNs, defaultBatchWorkers, v.GetDEK)
}

// PutIfAbsent creates a wrapped DEK using KV v2 check-and-set semantics.
func (v *HashiCorpVault) PutIfAbsent(ctx context.Context, patientURN string, encryptedDEK []byte) (bool, error) {
	path := v.secretPath(patientURN)
	data := map[string]interface{}{
		"dek": base64.StdEncoding.EncodeToString(encryptedDEK),
	}
	if _, err := v.client.KVv2(v.mount).Put(ctx, path, data, api.WithCheckAndSet(0)); err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "check-and-set") || strings.Contains(lower, "cas") {
			if _, getErr := v.GetDEK(ctx, patientURN); getErr != nil {
				if errors.Is(getErr, crypto.ErrKeyDestroyed) {
					return false, getErr
				}
				return false, errors.Wrap(getErr, "vault: hashicorp verify existing")
			}
			return false, nil
		}
		return false, errors.Wrap(err, "vault: hashicorp create")
	}
	return true, nil
}

// DeleteDEK permanently purges the secret payload and metadata from Vault KV v2,
// then writes a terminal tombstone so the key cannot be recreated.
func (v *HashiCorpVault) DeleteDEK(ctx context.Context, patientURN string) error {
	path := v.secretPath(patientURN)
	// DeleteMetadata permanently destroys all secret versions and metadata in KV v2.
	err := v.client.KVv2(v.mount).DeleteMetadata(ctx, path)
	if err != nil && !isVaultNotFound(err) {
		return errors.Wrap(err, "vault: hashicorp delete metadata")
	}
	data := map[string]interface{}{
		"dek": base64.StdEncoding.EncodeToString(crypto.DestroyedDEKMarker()),
	}
	if _, err := v.client.KVv2(v.mount).Put(ctx, path, data, api.WithCheckAndSet(0)); err != nil {
		return errors.Wrap(err, "vault: hashicorp tombstone")
	}
	return nil
}

// Close implements crypto.KeyVault; stateless HTTP client has no persistent connection to close.
func (v *HashiCorpVault) Close() error {
	return nil
}

// isVaultNotFound checks if a Vault API response error corresponds to HTTP 404 Not Found.
func isVaultNotFound(err error) bool {
	if err == nil {
		return false
	}
	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == http.StatusNotFound
	}
	return strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "secret not found")
}
