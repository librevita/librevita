package kv

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/vault/api"
)

// VaultStore is a Store backed by HashiCorp Vault / OpenBao KV v2.
type VaultStore struct {
	client *api.Client
	mount  string
	prefix string
}

// OpenVault initializes a KV v2 client.
func OpenVault(address, token, mount, prefix string) (*VaultStore, error) {
	if address == "" {
		return nil, errors.New("kv: vault address is required")
	}
	if token == "" {
		return nil, errors.New("kv: vault token is required")
	}
	if mount == "" {
		mount = "secret"
	}
	if prefix == "" {
		prefix = "librevita/keystore/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	cfg := api.DefaultConfig()
	cfg.Address = address
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "kv: vault client init")
	}
	client.SetToken(token)

	return &VaultStore{client: client, mount: mount, prefix: prefix}, nil
}

func (s *VaultStore) path(key string) string {
	return s.prefix + base64.RawURLEncoding.EncodeToString([]byte(key))
}

func (s *VaultStore) Get(ctx context.Context, key string) ([]byte, error) {
	secret, err := s.client.KVv2(s.mount).Get(ctx, s.path(key))
	if err != nil {
		if isVaultNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "kv: vault get")
	}
	if secret == nil || secret.Data == nil {
		return nil, ErrNotFound
	}
	raw, ok := secret.Data["value"].(string)
	if !ok || raw == "" {
		return nil, errors.New("kv: vault missing value")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.Wrap(err, "kv: vault decode")
	}
	return decoded, nil
}

func (s *VaultStore) GetMany(ctx context.Context, keys []string) (map[string]Result, error) {
	return batchGetWithWorkers(ctx, keys, defaultBatchWorkers, s.Get)
}

func (s *VaultStore) Put(ctx context.Context, key string, value []byte) error {
	data := map[string]interface{}{
		"value": base64.StdEncoding.EncodeToString(value),
	}
	if _, err := s.client.KVv2(s.mount).Put(ctx, s.path(key), data); err != nil {
		return errors.Wrap(err, "kv: vault put")
	}
	return nil
}

func (s *VaultStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	data := map[string]interface{}{
		"value": base64.StdEncoding.EncodeToString(value),
	}
	if _, err := s.client.KVv2(s.mount).Put(ctx, s.path(key), data, api.WithCheckAndSet(0)); err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "check-and-set") || strings.Contains(lower, "cas") {
			return false, nil
		}
		return false, errors.Wrap(err, "kv: vault create")
	}
	return true, nil
}

func (s *VaultStore) Delete(ctx context.Context, key string) error {
	err := s.client.KVv2(s.mount).DeleteMetadata(ctx, s.path(key))
	if err != nil && !isVaultNotFound(err) {
		return errors.Wrap(err, "kv: vault delete")
	}
	return nil
}

func (s *VaultStore) Shred(ctx context.Context, key string) error {
	return s.Delete(ctx, key)
}

func (s *VaultStore) ListPrefix(ctx context.Context, prefix string) ([]Entry, error) {
	dir := strings.TrimSuffix(s.prefix, "/")
	secret, err := s.client.Logical().ListWithContext(ctx, s.mount+"/metadata/"+dir)
	if err != nil {
		if isVaultNotFound(err) {
			return []Entry{}, nil
		}
		return nil, errors.Wrap(err, "kv: vault list")
	}
	names := vaultListKeys(secret)
	var entries []Entry
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name = strings.TrimSuffix(name, "/")
		raw, err := base64.RawURLEncoding.DecodeString(name)
		if err != nil {
			continue
		}
		logical := string(raw)
		if !strings.HasPrefix(logical, prefix) {
			continue
		}
		value, err := s.Get(ctx, logical)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		entries = append(entries, Entry{Key: logical, Value: value})
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

func vaultListKeys(secret *api.Secret) []string {
	if secret == nil || secret.Data == nil {
		return nil
	}
	raw, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		name, ok := item.(string)
		if !ok || name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func (s *VaultStore) Close() error {
	return nil
}

func isVaultNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, api.ErrSecretNotFound) {
		return true
	}
	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == http.StatusNotFound
	}
	return strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "secret not found")
}

var _ Shredder = (*VaultStore)(nil)
