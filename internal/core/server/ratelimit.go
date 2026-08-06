package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiter is a fixed-window limiter keyed by client. It is safe for
// concurrent use and suitable for a single-process monolith; a distributed
// deployment needs a shared counter (Redis, etc.).
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*windowBucket
}

type windowBucket struct {
	count int
	start time.Time
}

// NewRateLimiter allows limit requests per window per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*windowBucket),
	}
}

// Allow reports whether the key may proceed.
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.start) >= rl.window {
		b = &windowBucket{count: 0, start: now}
		rl.buckets[key] = b
	}

	// Keep the map bounded: drop buckets that have been idle for a window.
	if len(rl.buckets) > 4096 {
		for k, old := range rl.buckets {
			if now.Sub(old.start) >= rl.window && k != key {
				delete(rl.buckets, k)
			}
		}
	}

	b.count++
	return b.count <= rl.limit
}

// RateLimit guards a route by client IP with the shared limiter.
func RateLimit(rl *RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			if !rl.Allow(ctx.RealIP()) {
				return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests")
			}
			return next(ctx)
		}
	}
}
