package repository

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/clinic"
	"librevita.org/internal/domain/clinic/model"
)

type clinicRepository struct {
	client *ent.Client
}

// NewClinicRepository creates a clinic repository adapter.
func NewClinicRepository(client *ent.Client) model.Repository {
	return &clinicRepository{client: client}
}

func (r *clinicRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Clinic, error) {
	row, err := r.client.Clinic.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "clinic repository: get by id")
	}
	return toClinicDomain(row), nil
}

func (r *clinicRepository) GetBySlug(ctx context.Context, slug string) (*model.Clinic, error) {
	row, err := r.client.Clinic.Query().Where(clinic.SlugEQ(slug)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "clinic repository: get by slug")
	}
	return toClinicDomain(row), nil
}

func (r *clinicRepository) CreateShell(ctx context.Context, c *model.Clinic) (*model.Clinic, error) {
	create := r.client.Clinic.Create().
		SetID(c.ID).
		SetSlug(c.Slug).
		SetName(c.Name).
		SetCountry(c.Country).
		SetTimezone(c.Timezone)
	if c.TaxID != "" {
		create.SetTaxID(c.TaxID)
	}
	if c.Phone != "" {
		create.SetPhone(c.Phone)
	}
	if c.Email != "" {
		create.SetEmail(c.Email)
	}
	if c.Street != "" {
		create.SetStreet(c.Street)
	}
	if c.City != "" {
		create.SetCity(c.City)
	}
	if c.State != "" {
		create.SetState(c.State)
	}
	if c.PostalCode != "" {
		create.SetPostalCode(c.PostalCode)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errors.New("clinic repository: slug taken")
		}
		return nil, errors.Wrap(err, "clinic repository: create shell")
	}
	return toClinicDomain(row), nil
}

func (r *clinicRepository) MarkOnboarded(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := r.client.Clinic.UpdateOneID(id).SetOnboardedAt(at).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "clinic repository: mark onboarded")
	}
	return nil
}

func (r *clinicRepository) List(ctx context.Context) ([]*model.Clinic, error) {
	rows, err := r.client.Clinic.Query().Order(ent.Asc(clinic.FieldName)).All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "clinic repository: list")
	}
	out := make([]*model.Clinic, 0, len(rows))
	for _, row := range rows {
		out = append(out, toClinicDomain(row))
	}
	return out, nil
}

func toClinicDomain(row *ent.Clinic) *model.Clinic {
	if row == nil {
		return nil
	}
	return &model.Clinic{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		TaxID:       row.TaxID,
		Phone:       row.Phone,
		Email:       row.Email,
		Street:      row.Street,
		City:        row.City,
		State:       row.State,
		PostalCode:  row.PostalCode,
		Country:     row.Country,
		Timezone:    row.Timezone,
		OnboardedAt: row.OnboardedAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
