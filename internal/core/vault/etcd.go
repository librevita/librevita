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
	if err := ctx.Err(); err != nil {
		return err
	}
	k := v.key(patientURN)
	if _, err := v.GetDEK(ctx, patientURN); err != nil && !errors.Is(err, crypto.ErrKeyNotFound) {
		return err
	}
	if _, err := v.cli.Put(ctx, k, string(encryptedDEK)); err != nil {
		return fmt.Errorf("vault: etcd put: %w", err)
	}
	return nil
}

// GetDEK retrieves the encrypted DEK bytes for patientURN.
// Returns crypto.ErrKeyNotFound if key does not exist.
func (v *EtcdVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k := v.key(patientURN)
	resp, err := v.cli.Get(ctx, k)
	if err != nil {
		return nil, fmt.Errorf("vault: etcd get: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, crypto.ErrKeyNotFound
	}
	val := resp.Kvs[0].Value
	if crypto.IsDestroyedDEK(val) {
		return nil, crypto.ErrKeyDestroyed
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

// GetDEKs retrieves multiple wrapped DEKs with one transaction per bounded
// batch. Missing and destroyed keys are returned per item.
func (v *EtcdVault) GetDEKs(ctx context.Context, patientURNs []string) (map[string]crypto.DEKResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unique := uniqueURNs(patientURNs)
	results := make(map[string]crypto.DEKResult, len(unique))
	const batchSize = 64
	for start := 0; start < len(unique); start += batchSize {
		end := start + batchSize
		if end > len(unique) {
			end = len(unique)
		}
		ops := make([]clientv3.Op, 0, end-start)
		for _, urn := range unique[start:end] {
			ops = append(ops, clientv3.OpGet(v.key(urn)))
		}
		resp, err := v.cli.Txn(ctx).Then(ops...).Commit()
		if err != nil {
			return nil, fmt.Errorf("vault: etcd batch get: %w", err)
		}
		for i, urn := range unique[start:end] {
			rangeResp := resp.Responses[i].GetResponseRange()
			if rangeResp == nil || len(rangeResp.Kvs) == 0 {
				results[urn] = crypto.DEKResult{Err: crypto.ErrKeyNotFound}
				continue
			}
			value := rangeResp.Kvs[0].Value
			if crypto.IsDestroyedDEK(value) {
				results[urn] = crypto.DEKResult{Err: crypto.ErrKeyDestroyed}
				continue
			}
			copied := make([]byte, len(value))
			copy(copied, value)
			results[urn] = crypto.DEKResult{EncryptedDEK: copied}
		}
	}
	return results, nil
}

// PutIfAbsent stores a wrapped DEK without replacing an existing value.
func (v *EtcdVault) PutIfAbsent(ctx context.Context, patientURN string, encryptedDEK []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	k := v.key(patientURN)
	resp, err := v.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.Version(k), "=", int64(0))).
		Then(clientv3.OpPut(k, string(encryptedDEK))).
		Commit()
	if err != nil {
		return false, fmt.Errorf("vault: etcd create: %w", err)
	}
	return resp.Succeeded, nil
}

// DeleteDEK replaces the patient's DEK with a terminal tombstone, performing
// instant Crypto-Shredding without allowing recreation.
func (v *EtcdVault) DeleteDEK(ctx context.Context, patientURN string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := v.key(patientURN)
	if _, err := v.cli.Put(ctx, k, string(crypto.DestroyedDEKMarker())); err != nil {
		return fmt.Errorf("vault: etcd tombstone: %w", err)
	}
	return nil
}

// Close closes the etcd v3 client connection.
func (v *EtcdVault) Close() error {
	return v.cli.Close()
}
