// Package keystore wraps a kv.Store with DEK shredding semantics
// (tombstones, PutIfAbsent, batch get).
package keystore

import (
	"context"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/kv"
)

type store struct {
	inner kv.Store
}

// Wrap returns a crypto.KeyStore over inner. DeleteDEK writes a terminal
// tombstone after an optional physical shred.
func Wrap(inner kv.Store) crypto.KeyStore {
	return &store{inner: inner}
}

func (s *store) PutDEK(ctx context.Context, urn string, encryptedDEK []byte) error {
	if _, err := s.GetDEK(ctx, urn); err != nil && !errors.Is(err, crypto.ErrKeyNotFound) {
		return err
	}
	return s.inner.Put(ctx, urn, encryptedDEK)
}

func (s *store) GetDEK(ctx context.Context, urn string) ([]byte, error) {
	val, err := s.inner.Get(ctx, urn)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, crypto.ErrKeyNotFound
		}
		return nil, err
	}
	if crypto.IsDestroyedDEK(val) {
		return nil, crypto.ErrKeyDestroyed
	}
	return val, nil
}

func (s *store) GetDEKs(ctx context.Context, urns []string) (map[string]crypto.DEKResult, error) {
	raw, err := s.inner.GetMany(ctx, urns)
	if err != nil {
		return nil, err
	}
	out := make(map[string]crypto.DEKResult, len(raw))
	for urn, item := range raw {
		if item.Err != nil {
			if errors.Is(item.Err, kv.ErrNotFound) {
				out[urn] = crypto.DEKResult{Err: crypto.ErrKeyNotFound}
				continue
			}
			out[urn] = crypto.DEKResult{Err: item.Err}
			continue
		}
		if crypto.IsDestroyedDEK(item.Value) {
			out[urn] = crypto.DEKResult{Err: crypto.ErrKeyDestroyed}
			continue
		}
		out[urn] = crypto.DEKResult{EncryptedDEK: item.Value}
	}
	return out, nil
}

func (s *store) PutIfAbsent(ctx context.Context, urn string, encryptedDEK []byte) (bool, error) {
	_, err := s.GetDEK(ctx, urn)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, crypto.ErrKeyDestroyed) {
		return false, err
	}
	if !errors.Is(err, crypto.ErrKeyNotFound) {
		return false, err
	}
	return s.inner.PutIfAbsent(ctx, urn, encryptedDEK)
}

func (s *store) DeleteDEK(ctx context.Context, urn string) error {
	if shredder, ok := s.inner.(kv.Shredder); ok {
		if err := shredder.Shred(ctx, urn); err != nil {
			return err
		}
	}
	return s.inner.Put(ctx, urn, crypto.DestroyedDEKMarker())
}

func (s *store) Close() error {
	return s.inner.Close()
}

var (
	_ crypto.KeyStore            = (*store)(nil)
	_ crypto.BatchKeyStore       = (*store)(nil)
	_ crypto.ConditionalKeyStore = (*store)(nil)
)

// OpenBBolt opens a bbolt keystore at path (tests and helpers).
func OpenBBolt(path string) (crypto.KeyStore, error) {
	inner, err := kv.OpenBBolt(path)
	if err != nil {
		return nil, err
	}
	return Wrap(inner), nil
}
