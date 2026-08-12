package clinic

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"librevita.org/internal/domain/clinic/repository"
)

// clockCacheTTL bounds how stale a cached clinic profile can be. The
// profile changes rarely; a short TTL keeps the UI honest without a query
// per request.
const clockCacheTTL = time.Minute

// ClockProvider resolves the clinic profile (id, timezone, and the full
// row) into cached values shared by every request.
//
// The tenant model is single-clinic per installation (ADR-0001,
// docs/adr/0001-single-clinic-tenant.md): the profile is a singleton
// and clinic_id on clinical tables is future-proofing. This provider
// is the single resolution point -- a future multi-clinic mode swaps
// the singleton for the session's active clinic here, without touching
// the clinical schema.
type ClockProvider struct {
	db  *sql.DB
	mu  sync.Mutex
	row *repository.Clinic
	exp time.Time
}

// NewClockProvider is the Fx provider.
func NewClockProvider(db *sql.DB) *ClockProvider {
	return &ClockProvider{db: db}
}

// load returns the cached clinic row, refreshing it after the TTL.
// Systems without a clinic profile yet (pre-onboarding) fall back to the
// default zone and an empty id.
func (p *ClockProvider) load(ctx context.Context) (*repository.Clinic, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.row != nil && time.Now().Before(p.exp) {
		return p.row, nil
	}

	row, err := repository.New(p.db).GetClinic(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		row = repository.Clinic{Timezone: DefaultTimezone}
	}

	p.row = &row
	p.exp = time.Now().Add(clockCacheTTL)
	return p.row, nil
}

// Clock returns the clinic's clock. Systems without a clinic profile yet
// (pre-onboarding) fall back to the default zone.
func (p *ClockProvider) Clock(ctx context.Context) (*Clock, error) {
	row, err := p.load(ctx)
	if err != nil {
		return nil, err
	}
	return NewClock(row.Timezone), nil
}

// ClockFor returns the clock for the user's personal timezone, falling
// back to the clinic zone when tz is empty (the default preference).
func (p *ClockProvider) ClockFor(ctx context.Context, tz string) (*Clock, error) {
	if tz != "" {
		return NewClock(tz), nil
	}
	return p.Clock(ctx)
}

// ClinicID returns the clinic's id, cached briefly.
func (p *ClockProvider) ClinicID(ctx context.Context) (string, error) {
	row, err := p.load(ctx)
	if err != nil {
		return "", err
	}
	return row.ID.String(), nil
}

// Profile returns the clinic profile row, cached briefly.
func (p *ClockProvider) Profile(ctx context.Context) (*repository.Clinic, error) {
	return p.load(ctx)
}
