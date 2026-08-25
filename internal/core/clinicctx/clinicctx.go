// Package clinicctx carries the resolved clinic for the current request.
// Isolation, FLE AAD, ClockProvider, and CEL all read clinic_id from here.
// There is no tenant_id alias.
package clinicctx

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrMissingClinic is returned when a clinic-scoped operation runs without
// a clinic in context (and isolation has not been skipped).
var ErrMissingClinic = errors.New("clinicctx: clinic is required")

type contextKey int

const (
	clinicKey contextKey = iota
	skipKey
	apexKey
)

// Clinic is the request-scoped clinic profile (not the full Ent row).
type Clinic struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Timezone    string
	OnboardedAt *time.Time
}

// WithClinic stores c on ctx.
func WithClinic(ctx context.Context, c *Clinic) context.Context {
	return context.WithValue(ctx, clinicKey, c)
}

// FromContext returns the clinic attached to ctx, if any.
func FromContext(ctx context.Context) (*Clinic, bool) {
	c, ok := ctx.Value(clinicKey).(*Clinic)
	return c, ok && c != nil && c.ID != uuid.Nil
}

// ClinicID returns the clinic UUID in ctx.
func ClinicID(ctx context.Context) (uuid.UUID, bool) {
	c, ok := FromContext(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return c.ID, true
}

// MustClinicID returns the clinic UUID or ErrMissingClinic.
func MustClinicID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ClinicID(ctx)
	if !ok {
		return uuid.Nil, ErrMissingClinic
	}
	return id, nil
}

// WithSkipIsolation marks ctx as allowed to query without a clinic
// (migrations, seed, apex platform_users).
func WithSkipIsolation(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipKey, true)
}

// IsolationSkipped reports whether privacy/interceptors should no-op.
func IsolationSkipped(ctx context.Context) bool {
	skip, _ := ctx.Value(skipKey).(bool)
	return skip
}

// WithApex marks ctx as an apex (no clinic) request.
func WithApex(ctx context.Context) context.Context {
	return context.WithValue(ctx, apexKey, true)
}

// IsApex reports whether the request is on the installation apex.
func IsApex(ctx context.Context) bool {
	apex, _ := ctx.Value(apexKey).(bool)
	return apex
}

// ReservedSlugs cannot be used as clinic subdomains.
var ReservedSlugs = map[string]struct{}{
	"www":   {},
	"app":   {},
	"api":   {},
	"admin": {},
	"mail":  {},
}

// IsReservedSlug reports whether slug is blocked for clinic hosts.
func IsReservedSlug(slug string) bool {
	_, ok := ReservedSlugs[slug]
	return ok
}

// TestClinicID is a stable UUID for unit tests that need a clinic in context.
var TestClinicID = uuid.MustParse("01990000-0000-7000-8000-0000000000c1")

// WithTestClinic attaches a named onboarded clinic (slug "test") for tests.
func WithTestClinic(ctx context.Context) context.Context {
	now := time.Now()
	return WithClinic(ctx, &Clinic{
		ID:          TestClinicID,
		Slug:        "test",
		Name:        "Test Clinic",
		Timezone:    "America/Sao_Paulo",
		OnboardedAt: &now,
	})
}
