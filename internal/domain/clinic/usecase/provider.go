package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/domain/clinic/model"
)

// clockCacheTTL bounds how stale a cached clinic profile can be. The
// profile changes rarely; a short TTL keeps the UI honest without a query
// per request.
const clockCacheTTL = time.Minute

// ClockProvider resolves the clinic profile (id, timezone, and the full
// row) into cached values shared by every request.
type ClockProvider struct {
	repo model.Repository
	mu   sync.Mutex
	row  *model.Clinic
	exp  time.Time
}

// NewClockProvider is the Fx provider.
func NewClockProvider(repo model.Repository) *ClockProvider {
	return &ClockProvider{repo: repo}
}

// load returns the cached clinic row, refreshing it after the TTL.
// Systems without a clinic profile yet (pre-onboarding) fall back to the
// default zone and an empty id.
func (p *ClockProvider) load(ctx context.Context) (*model.Clinic, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.row != nil && time.Now().Before(p.exp) {
		return p.row, nil
	}

	row, err := p.repo.First(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &model.Clinic{Timezone: model.DefaultTimezone}
	}

	p.row = row
	p.exp = time.Now().Add(clockCacheTTL)
	return p.row, nil
}

// Clock returns the clinic's clock. Systems without a clinic profile yet
// (pre-onboarding) fall back to the default zone.
func (p *ClockProvider) Clock(ctx context.Context) (*model.Clock, error) {
	row, err := p.load(ctx)
	if err != nil {
		return nil, err
	}
	return model.NewClock(row.Timezone), nil
}

// ClockFor returns the clock for the user's personal timezone, falling
// back to the clinic zone when tz is empty (the default preference).
func (p *ClockProvider) ClockFor(ctx context.Context, tz string) (*model.Clock, error) {
	if tz != "" {
		return model.NewClock(tz), nil
	}
	return p.Clock(ctx)
}

// ClinicID returns the clinic's id, cached briefly.
func (p *ClockProvider) ClinicID(ctx context.Context) (string, error) {
	row, err := p.load(ctx)
	if err != nil {
		return "", err
	}
	if row.ID == uuid.Nil {
		return "", nil
	}
	return row.ID.String(), nil
}

// Profile returns the clinic profile row, cached briefly.
func (p *ClockProvider) Profile(ctx context.Context) (*model.Clinic, error) {
	return p.load(ctx)
}
