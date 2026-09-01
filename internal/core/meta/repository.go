// Package meta stores installation key-value metadata in a kv.Store.
package meta

import (
	"context"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/kv"
	"librevita.org/pkg/urn"
)

// Repository is the port for installation metadata.
type Repository interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

type repository struct {
	store kv.Store
}

// NewRepository wraps store.
func NewRepository(store kv.Store) Repository {
	return &repository{store: store}
}

func (r *repository) Get(ctx context.Context, key string) (string, error) {
	val, err := r.store.Get(ctx, urn.Meta(key))
	if err != nil {
		return "", err
	}
	return string(val), nil
}

func (r *repository) Put(ctx context.Context, key, value string) error {
	if key == "" {
		return errors.New("meta: key is required")
	}
	return r.store.Put(ctx, urn.Meta(key), []byte(value))
}

func (r *repository) Delete(ctx context.Context, key string) error {
	return r.store.Delete(ctx, urn.Meta(key))
}
