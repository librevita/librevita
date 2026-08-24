package fle

import (
	"context"
	"encoding/json"
	"sync"

	"librevita.org/internal/core/crypto"
)

type contextKey int

const (
	cleartextPayloadKey contextKey = iota
	searchableFieldKey
	aadKey
	decryptedRegistryKey
	encryptorKey
	encryptorResolverKey
	tenantIDKey
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

// ResolveAAD derives the Authenticated Associated Data (AAD) from the context.
// Priority:
// 1. Explicit custom AAD set via WithAAD
// 2. Tenant-scoped AAD ("urn:librevita:tenant:<tenant_id>") if WithTenantID is present
// 3. Default application AAD ("urn:librevita")
func ResolveAAD(ctx context.Context) []byte {
	if customAAD := AADFromContext(ctx); len(customAAD) > 0 {
		return customAAD
	}
	if tenantID, ok := TenantIDFromContext(ctx); ok && tenantID != "" {
		return []byte("urn:librevita:tenant:" + tenantID)
	}
	return []byte("urn:librevita")
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

// EncryptorResolver resolves a dynamic crypto.Encryptor from the request context.
type EncryptorResolver interface {
	ResolveEncryptor(ctx context.Context) (crypto.Encryptor, error)
}

// EncryptorResolverFunc is a function adapter implementing EncryptorResolver.
type EncryptorResolverFunc func(ctx context.Context) (crypto.Encryptor, error)

// ResolveEncryptor calls the underlying function.
func (f EncryptorResolverFunc) ResolveEncryptor(ctx context.Context) (crypto.Encryptor, error) {
	return f(ctx)
}

// WithEncryptor attaches a dynamic crypto.Encryptor instance to the context.
func WithEncryptor(ctx context.Context, enc crypto.Encryptor) context.Context {
	return context.WithValue(ctx, encryptorKey, enc)
}

// EncryptorFromContext retrieves the dynamic crypto.Encryptor from context if present.
func EncryptorFromContext(ctx context.Context) (crypto.Encryptor, bool) {
	enc, ok := ctx.Value(encryptorKey).(crypto.Encryptor)
	return enc, ok && enc != nil
}

// WithEncryptorResolver attaches a dynamic EncryptorResolver to the context.
func WithEncryptorResolver(ctx context.Context, resolver EncryptorResolver) context.Context {
	return context.WithValue(ctx, encryptorResolverKey, resolver)
}

// EncryptorResolverFromContext retrieves the EncryptorResolver from context if present.
func EncryptorResolverFromContext(ctx context.Context) (EncryptorResolver, bool) {
	r, ok := ctx.Value(encryptorResolverKey).(EncryptorResolver)
	return r, ok && r != nil
}

// WithTenantID attaches a dynamic tenant ID string to the context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantIDFromContext retrieves the tenant ID string from context if present.
func TenantIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(tenantIDKey).(string)
	return val, ok && val != ""
}

// ResolveEncryptor dynamically resolves the active crypto.Encryptor using:
// 1. Direct Encryptor in context (via WithEncryptor)
// 2. EncryptorResolver in context (via WithEncryptorResolver)
// 3. Fallback to the defaultEncryptor configured at client startup
func ResolveEncryptor(ctx context.Context, defaultEnc crypto.Encryptor) (crypto.Encryptor, error) {
	if enc, ok := EncryptorFromContext(ctx); ok {
		return enc, nil
	}
	if resolver, ok := EncryptorResolverFromContext(ctx); ok {
		enc, err := resolver.ResolveEncryptor(ctx)
		if err != nil {
			return nil, err
		}
		if enc != nil {
			return enc, nil
		}
	}
	return defaultEnc, nil
}
