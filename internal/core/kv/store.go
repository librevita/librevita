// Package kv provides a generic key-value Store used by keystore, meta, and
// sessions. Adapters do not implement DEK shredding; that lives in
// internal/core/keystore.
package kv

import (
	"context"

	"github.com/cockroachdb/errors"
)

var (
	// ErrNotFound indicates the key does not exist.
	ErrNotFound = errors.New("kv: key not found")

	// ErrClosed indicates operations were attempted on a closed Store.
	ErrClosed = errors.New("kv: store is closed")
)

// Entry is one key/value pair returned by ListPrefix.
type Entry struct {
	Key   string
	Value []byte
}

// Result is the outcome for one key in a batch GetMany.
type Result struct {
	Value []byte
	Err   error
}

// Store is a namespaced byte map. Keys are logical URNs; adapters may encode
// them for the backing engine.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	GetMany(ctx context.Context, keys []string) (map[string]Result, error)
	Put(ctx context.Context, key string, value []byte) error
	PutIfAbsent(ctx context.Context, key string, value []byte) (created bool, err error)
	Delete(ctx context.Context, key string) error
	ListPrefix(ctx context.Context, prefix string) ([]Entry, error)
	Close() error
}

// Shredder is an optional extension for engines that keep version history.
// Keystore uses it before writing a crypto-shred tombstone.
type Shredder interface {
	Shred(ctx context.Context, key string) error
}
