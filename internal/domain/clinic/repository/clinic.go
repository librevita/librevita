package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/internal/domain/clinic/model"
)

type clinicRepository struct {
	client *ent.Client
}

// NewClinicRepository creates a clinic repository adapter.
func NewClinicRepository(client *ent.Client) model.Repository {
	return &clinicRepository{client: client}
}

func (r *clinicRepository) First(ctx context.Context) (*model.Clinic, error) {
	row, err := r.client.Clinic.Query().First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("clinic repository: first: %w", err)
	}
	return toClinicDomain(row), nil
}

func (r *clinicRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Clinic, error) {
	row, err := r.client.Clinic.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("clinic repository: get by id: %w", err)
	}
	return toClinicDomain(row), nil
}

func toClinicDomain(row *ent.Clinic) *model.Clinic {
	if row == nil {
		return nil
	}
	return &model.Clinic{
		ID:         row.ID,
		Name:       row.Name,
		TaxID:      row.TaxID,
		Phone:      row.Phone,
		Email:      row.Email,
		Street:     row.Street,
		City:       row.City,
		State:      row.State,
		PostalCode: row.PostalCode,
		Country:    row.Country,
		Timezone:   row.Timezone,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
