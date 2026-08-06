package clinic

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"librevita.org/internal/domain/clinic/repository"
)

// clockCacheTTL bounds how stale a cached clinic timezone can be. The
// profile changes rarely; a short TTL keeps the UI honest without a query
// per request.
const clockCacheTTL = time.Minute

// ClockProvider resolves the clinic's timezone into a Clock.
type ClockProvider struct {
	db   *sql.DB
	mu   sync.Mutex
	zone string
	exp  time.Time
}

// NewClockProvider is the Fx provider.
func NewClockProvider(db *sql.DB) *ClockProvider {
	return &ClockProvider{db: db}
}

// Clock returns the clinic's clock, cached briefly. Systems without a
// clinic profile yet (pre-onboarding) fall back to the default zone.
func (p *ClockProvider) Clock(ctx context.Context) (*Clock, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.zone != "" && now.Before(p.exp) {
		return NewClock(p.zone), nil
	}

	zone := DefaultTimezone
	row, err := repository.New(p.db).GetClinic(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		zone = row.Timezone
	}

	p.zone = zone
	p.exp = now.Add(clockCacheTTL)
	return NewClock(zone), nil
}
