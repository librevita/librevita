package crypto

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"
)

type requestKeyCacheKey struct{}

type requestKeyCache struct {
	mu   sync.RWMutex
	deks map[string][]byte
	sf   singleflight.Group
}

// WithRequestKeyCache installs a request-scoped cache for unwrapped DEKs.
// The cache is deliberately attached to a request context instead of the
// Engine so one request cannot retain key material for another request.
func WithRequestKeyCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(requestKeyCacheKey{}).(*requestKeyCache); ok {
		return ctx
	}
	return context.WithValue(ctx, requestKeyCacheKey{}, &requestKeyCache{
		deks: make(map[string][]byte),
	})
}

// HasRequestKeyCache reports whether ctx already owns a request-scoped key
// cache.
func HasRequestKeyCache(ctx context.Context) bool {
	_, ok := ctx.Value(requestKeyCacheKey{}).(*requestKeyCache)
	return ok
}

// ClearRequestKeyCache zeroes all cached DEKs and releases their references.
// Callers owning the request context should defer this immediately after
// WithRequestKeyCache.
func ClearRequestKeyCache(ctx context.Context) {
	cache, _ := ctx.Value(requestKeyCacheKey{}).(*requestKeyCache)
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key, dek := range cache.deks {
		ZeroBytes(dek)
		delete(cache.deks, key)
	}
}

func requestCacheFromContext(ctx context.Context) *requestKeyCache {
	cache, _ := ctx.Value(requestKeyCacheKey{}).(*requestKeyCache)
	return cache
}

func (c *requestKeyCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	dek, ok := c.deks[key]
	if !ok {
		return nil, false
	}
	return cloneBytes(dek), true
}

func (c *requestKeyCache) put(key string, dek []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, ok := c.deks[key]; ok {
		ZeroBytes(previous)
	}
	c.deks[key] = cloneBytes(dek)
}

func cacheDEK(ctx context.Context, key string, dek []byte) {
	if cache := requestCacheFromContext(ctx); cache != nil {
		cache.put(key, dek)
	}
}

func forgetDEK(ctx context.Context, key string) {
	cache := requestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if dek, ok := cache.deks[key]; ok {
		ZeroBytes(dek)
		delete(cache.deks, key)
	}
}

func (e *Engine) cachedDEK(ctx context.Context, key string, load func() ([]byte, error)) ([]byte, error) {
	cache := requestCacheFromContext(ctx)
	if cache == nil {
		e.recordKeyStoreGet()
		return load()
	}
	if dek, ok := cache.get(key); ok {
		e.recordCacheHit()
		return dek, nil
	}
	e.recordCacheMiss()
	return e.loadThroughSingleflight(cache, key, load)
}

func (e *Engine) recordKeyStoreGet() {
	if e.metrics != nil {
		e.metrics.keyStoreGet.Add(1)
	}
}

func (e *Engine) recordCacheHit() {
	if e.metrics != nil {
		e.metrics.cacheHit.Add(1)
	}
}

func (e *Engine) recordCacheMiss() {
	if e.metrics != nil {
		e.metrics.cacheMiss.Add(1)
	}
}

func (e *Engine) loadThroughSingleflight(cache *requestKeyCache, key string, load func() ([]byte, error)) ([]byte, error) {
	value, err, _ := cache.sf.Do(key, func() (any, error) {
		return e.loadAndStoreDEK(cache, key, load)
	})
	if err != nil {
		return nil, err
	}
	dek, ok := value.([]byte)
	if !ok || len(dek) != SizeDEK {
		return nil, ErrInvalidDEK
	}
	return cloneBytes(dek), nil
}

func (e *Engine) loadAndStoreDEK(cache *requestKeyCache, key string, load func() ([]byte, error)) (any, error) {
	if dek, ok := cache.get(key); ok {
		return dek, nil
	}
	e.recordKeyStoreGet()
	dek, err := load()
	if err != nil {
		return nil, err
	}
	if len(dek) != SizeDEK {
		ZeroBytes(dek)
		return nil, ErrInvalidDEK
	}
	cache.put(key, dek)
	ZeroBytes(dek)
	stored, ok := cache.get(key)
	if !ok {
		return nil, ErrInvalidDEK
	}
	return stored, nil
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
