package model

import (
	"context"
	"regexp"
	"strings"
	"time"

	"librevita.org/pkg/ident"
)

// clinicDomainRE is the DNS-safe hostname or domain name for the clinic.
var clinicDomainRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidDomain reports whether domain is a valid DNS-safe clinic domain.
func ValidDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" || len(d) > 253 {
		return false
	}
	return clinicDomainRE.MatchString(d)
}

// ValidSlug reports whether slug is a valid clinic identifier (alias for ValidDomain).
func ValidSlug(slug string) bool {
	return ValidDomain(slug)
}

// Clinic is the domain model representing a clinic profile.
type Clinic struct {
	ID          ident.ClinicID
	Domain      string
	Slug        string // deprecated: alias for Domain
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
	GetByID(ctx context.Context, id ident.ClinicID) (*Clinic, error)
	GetByDomain(ctx context.Context, domain string) (*Clinic, error)
	GetBySlug(ctx context.Context, slug string) (*Clinic, error)
	CreateShell(ctx context.Context, c *Clinic) (*Clinic, error)
	MarkOnboarded(ctx context.Context, id ident.ClinicID, at time.Time) error
	List(ctx context.Context) ([]*Clinic, error)
}
