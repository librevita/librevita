package model

import (
	"context"
	"regexp"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
)

// clinicSlugRE is the DNS-safe hostname label used as the clinic subdomain.
var clinicSlugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidSlug reports whether slug is a DNS-safe, non-reserved clinic label.
func ValidSlug(slug string) bool {
	if !clinicSlugRE.MatchString(slug) {
		return false
	}
	return !clinicctx.IsReservedSlug(slug)
}

// Clinic is the domain model representing a clinic profile.
type Clinic struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	TaxID       string
	Phone       string
	Email       string
	Street      string
	City        string
	State       string
	PostalCode  string
	Country     string
	Timezone    string
	OnboardedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Onboarded reports whether clinic /setup has completed.
func (c *Clinic) Onboarded() bool {
	return c != nil && c.OnboardedAt != nil && !c.OnboardedAt.IsZero()
}

// Repository defines the storage contract for clinic data.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Clinic, error)
	GetBySlug(ctx context.Context, slug string) (*Clinic, error)
	CreateShell(ctx context.Context, c *Clinic) (*Clinic, error)
	MarkOnboarded(ctx context.Context, id uuid.UUID, at time.Time) error
	List(ctx context.Context) ([]*Clinic, error)
}
