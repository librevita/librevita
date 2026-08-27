package crypto

import "sync/atomic"

type keyMetrics struct {
	vaultGet      atomic.Uint64
	vaultBatchGet atomic.Uint64
	cacheHit      atomic.Uint64
	cacheMiss     atomic.Uint64
}

// KeyMetricsSnapshot is a point-in-time view of key resolution counters.
type KeyMetricsSnapshot struct {
	VaultGet      uint64
	VaultBatchGet uint64
	CacheHit      uint64
	CacheMiss     uint64
}

// KeyMetrics returns counters useful for validating vault query reduction.
func (e *Engine) KeyMetrics() KeyMetricsSnapshot {
	if e == nil || e.metrics == nil {
		return KeyMetricsSnapshot{}
	}
	return KeyMetricsSnapshot{
		VaultGet:      e.metrics.vaultGet.Load(),
		VaultBatchGet: e.metrics.vaultBatchGet.Load(),
		CacheHit:      e.metrics.cacheHit.Load(),
		CacheMiss:     e.metrics.cacheMiss.Load(),
	}
}
