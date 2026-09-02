package usecase

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/pkg/flow"
	"librevita.org/pkg/ident"
)

var (
	ErrPlatformExists      = errors.New("clinic: platform already bootstrapped")
	ErrInvalidPlatformCred = errors.New("clinic: invalid email or password")
	ErrSlugTaken           = errors.New("clinic: slug already in use")
	ErrInvalidSlug         = errors.New("clinic: invalid slug")
)

// PlatformService bootstraps platform operators and provisions clinic shells.
type PlatformService struct {
	users   repository.PlatformUserRepository
	clinics model.Repository
	engine  *crypto.Engine
	dummy   string
}

// NewPlatformService is the Fx provider.
func NewPlatformService(users repository.PlatformUserRepository, clinics model.Repository, engine *crypto.Engine) *PlatformService {
	dummy, _ := auth.HashPassword("dummy-password-for-timing")
	return &PlatformService{users: users, clinics: clinics, engine: engine, dummy: dummy}
}

// HasOperators reports whether any platform_users row exists.
func (s *PlatformService) HasOperators(ctx context.Context) (bool, error) {
	n, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Bootstrap creates the first platform operator. Fails if one already exists.
func (s *PlatformService) Bootstrap(ctx context.Context, name, email, password string) (*auth.Principal, string, error) {
	exists, err := s.HasOperators(ctx)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", ErrPlatformExists
	}
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || !strings.Contains(email, "@") || len(password) < 8 {
		return nil, "", errors.New("clinic: invalid bootstrap input")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, "", err
	}
	userID := ident.New[ident.PlatformUserID]()
	u, err := s.users.Create(ctx, &repository.PlatformUser{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Active:       true,
	})
	if err != nil {
		return nil, "", err
	}
	p := &auth.Principal{ID: u.ID.String(), Email: u.Email, Name: u.DisplayName, Platform: true}
	return p, "", nil
}

// Login authenticates a platform operator.
func (s *PlatformService) Login(ctx context.Context, email, password string) (*auth.Principal, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.Active {
		if s.dummy != "" {
			_, _ = auth.VerifyPassword(s.dummy, password)
		}
		return nil, ErrInvalidPlatformCred
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, password)
	if err != nil || !ok {
		return nil, ErrInvalidPlatformCred
	}
	return &auth.Principal{ID: u.ID.String(), Email: u.Email, Name: u.DisplayName, Platform: true}, nil
}

// ProvisionInput is the clinic shell created on the apex.
type ProvisionInput struct {
	Name     string
	Slug     string
	TaxID    string
	Phone    string
	Email    string
	Street   string
	City     string
	State    string
	Postal   string
	Country  string
	Timezone string
}

// Provision creates a clinic shell (onboarded_at null) and its Clinic DEK.
func (s *PlatformService) Provision(ctx context.Context, in ProvisionInput) (*model.Clinic, error) {
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	name := strings.TrimSpace(in.Name)
	tz := strings.TrimSpace(in.Timezone)
	if tz == "" {
		tz = model.DefaultTimezone
	}
	country := strings.ToUpper(strings.TrimSpace(in.Country))
	if country == "" {
		country = "BR"
	}

	var shell *model.Clinic
	var clinicID ident.ClinicID

	err := flow.New().
		Step("validate slug", func() error {
			if !model.ValidSlug(slug) {
				return ErrInvalidSlug
			}
			return nil
		}).
		Step("validate name", func() error {
			if name == "" {
				return errors.New("clinic: name is required")
			}
			return nil
		}).
		Step("generate clinic id", func() error {
			clinicID = ident.New[ident.ClinicID]()
			return nil
		}).
		Step("create clinic shell", func() error {
			var cerr error
			shell, cerr = s.clinics.CreateShell(ctx, &model.Clinic{
				ID:         clinicID,
				Slug:       slug,
				Name:       name,
				TaxID:      strings.TrimSpace(in.TaxID),
				Phone:      strings.TrimSpace(in.Phone),
				Email:      strings.ToLower(strings.TrimSpace(in.Email)),
				Street:     strings.TrimSpace(in.Street),
				City:       strings.TrimSpace(in.City),
				State:      strings.TrimSpace(in.State),
				PostalCode: strings.TrimSpace(in.Postal),
				Country:    country,
				Timezone:   tz,
			})
			if cerr != nil {
				if strings.Contains(cerr.Error(), "slug taken") {
					return ErrSlugTaken
				}
				return cerr
			}
			return nil
		}).
		Step("ensure clinic dek", func() error {
			if s.engine != nil {
				if _, err := s.engine.EnsureClinicDEK(ctx, shell.ID); err != nil {
					return errors.Wrap(err, "clinic: ensure dek")
				}
			}
			return nil
		}).
		Err()

	if err != nil {
		return nil, err
	}
	return shell, nil
}

// ListClinics returns provisioned clinic shells.
func (s *PlatformService) ListClinics(ctx context.Context) ([]*model.Clinic, error) {
	return s.clinics.List(ctx)
}
