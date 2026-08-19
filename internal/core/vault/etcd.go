package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"librevita.org/internal/core/crypto"
)

// EtcdVault implements crypto.KeyVault using etcd v3.
type EtcdVault struct {
	cli    *clientv3.Client
	prefix string
}

// NewEtcdVault creates a new etcd KeyVault client connected to endpoints.
func NewEtcdVault(endpointsStr, prefix string) (*EtcdVault, error) {
	if endpointsStr == "" {
		return nil, errors.New("vault: etcd endpoints are required")
	}
	if prefix == "" {
		prefix = "/librevita/keys/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	endpoints := strings.Split(endpointsStr, ",")
	for i, ep := range endpoints {
		endpoints[i] = strings.TrimSpace(ep)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("vault: etcd client init: %w", err)
	}

	return &EtcdVault{
		cli:    cli,
		prefix: prefix,
	}, nil
}

func (v *EtcdVault) key(patientURN string) string {
	return v.prefix + patientURN
}

// PutDEK stores the encrypted DEK bytes indexed by patientURN.
func (v *EtcdVault) PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error {
	k := v.key(patientURN)
	if _, err := v.cli.Put(ctx, k, string(encryptedDEK)); err != nil {
		return fmt.Errorf("vault: etcd put: %w", err)
	}
	return nil
}

// GetDEK retrieves the encrypted DEK bytes for patientURN.
// Returns crypto.ErrKeyNotFound if key does not exist.
func (v *EtcdVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	k := v.key(patientURN)
	resp, err := v.cli.Get(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("vault: etcd get: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, crypto.ErrKeyNotFound
	}
	val := resp.Kvs[0].Value
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

// DeleteDEK removes the patient's DEK from etcd, performing instant Crypto-Shredding.
func (v *EtcdVault) DeleteDEK(ctx context.Context, patientURN string) error {
	k := v.key(patientURN)
	if _, err := v.cli.Delete(ctx, k); err != nil {
		return fmt.Errorf("vault: etcd delete: %w", err)
	}
	return nil
}

// Close closes the etcd v3 client connection.
func (v *EtcdVault) Close() error {
	return v.cli.Close()
}
