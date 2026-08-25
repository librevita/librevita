package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/domain/clinic/model"
)

// clockCacheTTL bounds how stale a cached clinic profile can be.
const clockCacheTTL = time.Minute

type cachedClinic struct {
	row *model.Clinic
	exp time.Time
}

// ClockProvider resolves the request clinic (id, timezone, and the full
// row) into cached values keyed by clinic id.
type ClockProvider struct {
	repo  model.Repository
	mu    sync.Mutex
	cache map[uuid.UUID]cachedClinic
}

// NewClockProvider is the Fx provider.
func NewClockProvider(repo model.Repository) *ClockProvider {
	return &ClockProvider{repo: repo, cache: make(map[uuid.UUID]cachedClinic)}
}

func (p *ClockProvider) loadByID(ctx context.Context, id uuid.UUID) (*model.Clinic, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.cache[id]; ok && time.Now().Before(entry.exp) {
		return entry.row, nil
	}

	row, err := p.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &model.Clinic{ID: id, Timezone: model.DefaultTimezone}
	}
	p.cache[id] = cachedClinic{row: row, exp: time.Now().Add(clockCacheTTL)}
	return row, nil
}

func (p *ClockProvider) fromContext(ctx context.Context) (*model.Clinic, bool) {
	c, ok := clinicctx.FromContext(ctx)
	if !ok {
		return nil, false
	}
	return &model.Clinic{
		ID:          c.ID,
		Slug:        c.Slug,
		Name:        c.Name,
		Timezone:    c.Timezone,
		OnboardedAt: c.OnboardedAt,
	}, true
}

// Clock returns the clinic's clock. Apex and pre-onboarding fall back
// to the default zone.
func (p *ClockProvider) Clock(ctx context.Context) (*model.Clock, error) {
	if c, ok := p.fromContext(ctx); ok {
		tz := c.Timezone
		if tz == "" {
			tz = model.DefaultTimezone
		}
		return model.NewClock(tz), nil
	}
	return model.NewClock(model.DefaultTimezone), nil
}

// ClockFor returns the clock for the user's personal timezone, falling
// back to the clinic zone when tz is empty (the default preference).
func (p *ClockProvider) ClockFor(ctx context.Context, tz string) (*model.Clock, error) {
	if tz != "" {
		return model.NewClock(tz), nil
	}
	return p.Clock(ctx)
}

// ClinicID returns the clinic's id from request context.
func (p *ClockProvider) ClinicID(ctx context.Context) (string, error) {
	if id, ok := clinicctx.ClinicID(ctx); ok {
		return id.String(), nil
	}
	return "", nil
}

// Profile returns the full clinic row, cached by id. Apex returns an
// empty profile with the default timezone.
func (p *ClockProvider) Profile(ctx context.Context) (*model.Clinic, error) {
	id, ok := clinicctx.ClinicID(ctx)
	if !ok {
		return &model.Clinic{Timezone: model.DefaultTimezone}, nil
	}
	return p.loadByID(ctx, id)
}
