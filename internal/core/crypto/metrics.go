package crypto

import "sync/atomic"

type keyMetrics struct {
	keyStoreGet      atomic.Uint64
	keyStoreBatchGet atomic.Uint64
	cacheHit         atomic.Uint64
	cacheMiss        atomic.Uint64
}

// KeyMetricsSnapshot is a point-in-time view of key resolution counters.
type KeyMetricsSnapshot struct {
	KeyStoreGet      uint64
	KeyStoreBatchGet uint64
	CacheHit         uint64
	CacheMiss        uint64
}

// KeyMetrics returns counters useful for validating keystore query reduction.
func (e *Engine) KeyMetrics() KeyMetricsSnapshot {
	if e == nil || e.metrics == nil {
		return KeyMetricsSnapshot{}
	}
	return KeyMetricsSnapshot{
		KeyStoreGet:      e.metrics.keyStoreGet.Load(),
		KeyStoreBatchGet: e.metrics.keyStoreBatchGet.Load(),
		CacheHit:         e.metrics.cacheHit.Load(),
		CacheMiss:        e.metrics.cacheMiss.Load(),
	}
}
