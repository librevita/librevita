package fle

import (
	"context"
	"encoding/json"
	"sync"
)

type contextKey int

const (
	cleartextPayloadKey contextKey = iota
	searchableFieldKey
	aadKey
	decryptedRegistryKey
)

type searchableField struct {
	domain string
	value  string
}

type decryptedRegistry struct {
	mu       sync.RWMutex
	payloads map[any][]byte
}

func newDecryptedRegistry() *decryptedRegistry {
	return &decryptedRegistry{
		payloads: make(map[any][]byte),
	}
}

func (r *decryptedRegistry) set(key any, val []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads[key] = val
}

func (r *decryptedRegistry) get(key any) ([]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	val, ok := r.payloads[key]
	return val, ok
}

// WithCleartextPayload attaches a cleartext payload to the context for encryption by the mutation hook.
func WithCleartextPayload(ctx context.Context, payload any) context.Context {
	return context.WithValue(ctx, cleartextPayloadKey, payload)
}

// CleartextPayloadFromContext retrieves the cleartext payload from the context.
func CleartextPayloadFromContext(ctx context.Context) (any, bool) {
	val := ctx.Value(cleartextPayloadKey)
	if val == nil {
		return nil, false
	}
	return val, true
}

// WithSearchableField attaches a searchable field (domain + value) for blind index computation by the mutation hook.
func WithSearchableField(ctx context.Context, domain, value string) context.Context {
	return context.WithValue(ctx, searchableFieldKey, searchableField{
		domain: domain,
		value:  value,
	})
}

// SearchableFieldFromContext retrieves the searchable field from the context.
func SearchableFieldFromContext(ctx context.Context) (domain, value string, ok bool) {
	val, exists := ctx.Value(searchableFieldKey).(searchableField)
	if !exists {
		return "", "", false
	}
	return val.domain, val.value, true
}

// WithAAD attaches Authenticated Associated Data (AAD) to the context for AEAD encryption/decryption.
func WithAAD(ctx context.Context, aad []byte) context.Context {
	return context.WithValue(ctx, aadKey, aad)
}

// AADFromContext retrieves AAD from the context.
func AADFromContext(ctx context.Context) []byte {
	val, ok := ctx.Value(aadKey).([]byte)
	if !ok {
		return nil
	}
	return val
}

// WithDecryptedRegistry ensures a registry exists in the context to store post-query decrypted payloads.
func WithDecryptedRegistry(ctx context.Context) context.Context {
	if ctx.Value(decryptedRegistryKey) != nil {
		return ctx
	}
	return context.WithValue(ctx, decryptedRegistryKey, newDecryptedRegistry())
}

// StoreDecrypted stores decrypted bytes associated with an entity key in the context registry.
func StoreDecrypted(ctx context.Context, key any, payload []byte) {
	reg, ok := ctx.Value(decryptedRegistryKey).(*decryptedRegistry)
	if !ok || reg == nil {
		return
	}
	reg.set(key, payload)
}

// GetDecrypted retrieves decrypted bytes for an entity key from the context registry.
func GetDecrypted(ctx context.Context, key any) ([]byte, bool) {
	reg, ok := ctx.Value(decryptedRegistryKey).(*decryptedRegistry)
	if !ok || reg == nil {
		return nil, false
	}
	return reg.get(key)
}

// GetDecryptedInto unmarshals decrypted JSON payload for an entity key into target.
func GetDecryptedInto(ctx context.Context, key any, target any) (bool, error) {
	raw, ok := GetDecrypted(ctx, key)
	if !ok || len(raw) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return true, err
	}
	return true, nil
}
