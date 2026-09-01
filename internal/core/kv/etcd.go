package kv

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStore is a Store backed by etcd v3 under a key prefix.
type EtcdStore struct {
	cli    *clientv3.Client
	prefix string
}

// OpenEtcd connects to the given endpoints.
func OpenEtcd(endpointsStr, prefix string) (*EtcdStore, error) {
	if endpointsStr == "" {
		return nil, errors.New("kv: etcd endpoints are required")
	}
	if prefix == "" {
		return nil, errors.New("kv: etcd prefix is required")
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
		return nil, errors.Wrap(err, "kv: etcd client init")
	}

	return &EtcdStore{cli: cli, prefix: prefix}, nil
}

func (s *EtcdStore) key(logical string) string {
	return s.prefix + logical
}

func (s *EtcdStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp, err := s.cli.Get(ctx, s.key(key))
	if err != nil {
		return nil, errors.Wrap(err, "kv: etcd get")
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrNotFound
	}
	return bytesClone(resp.Kvs[0].Value), nil
}

func (s *EtcdStore) GetMany(ctx context.Context, keys []string) (map[string]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unique := uniqueKeys(keys)
	results := make(map[string]Result, len(unique))
	const batchSize = 64
	for start := 0; start < len(unique); start += batchSize {
		end := start + batchSize
		if end > len(unique) {
			end = len(unique)
		}
		ops := make([]clientv3.Op, 0, end-start)
		for _, key := range unique[start:end] {
			ops = append(ops, clientv3.OpGet(s.key(key)))
		}
		resp, err := s.cli.Txn(ctx).Then(ops...).Commit()
		if err != nil {
			return nil, errors.Wrap(err, "kv: etcd batch get")
		}
		for i, key := range unique[start:end] {
			rangeResp := resp.Responses[i].GetResponseRange()
			if rangeResp == nil || len(rangeResp.Kvs) == 0 {
				results[key] = Result{Err: ErrNotFound}
				continue
			}
			results[key] = Result{Value: bytesClone(rangeResp.Kvs[0].Value)}
		}
	}
	return results, nil
}

func (s *EtcdStore) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.cli.Put(ctx, s.key(key), string(value)); err != nil {
		return errors.Wrap(err, "kv: etcd put")
	}
	return nil
}

func (s *EtcdStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	k := s.key(key)
	resp, err := s.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.Version(k), "=", int64(0))).
		Then(clientv3.OpPut(k, string(value))).
		Commit()
	if err != nil {
		return false, errors.Wrap(err, "kv: etcd create")
	}
	return resp.Succeeded, nil
}

func (s *EtcdStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.cli.Delete(ctx, s.key(key)); err != nil {
		return errors.Wrap(err, "kv: etcd delete")
	}
	return nil
}

func (s *EtcdStore) ListPrefix(ctx context.Context, prefix string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp, err := s.cli.Get(ctx, s.key(prefix), clientv3.WithPrefix())
	if err != nil {
		return nil, errors.Wrap(err, "kv: etcd list")
	}
	entries := make([]Entry, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		logical, ok := strings.CutPrefix(string(kv.Key), s.prefix)
		if !ok {
			continue
		}
		entries = append(entries, Entry{Key: logical, Value: bytesClone(kv.Value)})
	}
	return entries, nil
}

func (s *EtcdStore) Close() error {
	return s.cli.Close()
}
