package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/specialty"
	"librevita.org/ent/user"
	usermodel "librevita.org/internal/domain/user/model"
)

type specialtyRepository struct {
	client *ent.Client
}

// NewSpecialtyRepository creates a specialty repository adapter.
func NewSpecialtyRepository(client *ent.Client) usermodel.SpecialtyRepository {
	return &specialtyRepository{client: client}
}

func (r *specialtyRepository) ListByClinic(ctx context.Context, clinicID uuid.UUID) ([]usermodel.Specialty, error) {
	specialties, err := r.client.Specialty.Query().
		Where(specialty.ClinicIDEQ(clinicID)).
		Order(ent.Asc(specialty.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("specialty repository: list: %w", err)
	}

	rows := make([]usermodel.Specialty, 0, len(specialties))
	for _, sp := range specialties {
		rows = append(rows, *toSpecialtyDomain(sp))
	}
	return rows, nil
}

func (r *specialtyRepository) ListPageByClinic(ctx context.Context, clinicID uuid.UUID, limit, offset int) ([]usermodel.Specialty, int64, error) {
	specialties, err := r.client.Specialty.Query().
		Where(specialty.ClinicIDEQ(clinicID)).
		Order(ent.Asc(specialty.FieldName)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("specialty repository: list page: %w", err)
	}

	total, err := r.client.Specialty.Query().Where(specialty.ClinicIDEQ(clinicID)).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("specialty repository: count: %w", err)
	}

	rows := make([]usermodel.Specialty, 0, len(specialties))
	for _, sp := range specialties {
		rows = append(rows, *toSpecialtyDomain(sp))
	}
	return rows, int64(total), nil
}

func (r *specialtyRepository) Create(ctx context.Context, sp *usermodel.Specialty) (*usermodel.Specialty, error) {
	exists, err := r.client.Specialty.Query().
		Where(
			specialty.ClinicIDEQ(sp.ClinicID),
			specialty.NameEqualFold(sp.Name),
		).
		Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("specialty repository: check duplicate: %w", err)
	}
	if exists {
		return nil, usermodel.ErrDuplicateSpecialty
	}

	saved, err := r.client.Specialty.Create().
		SetID(sp.ID).
		SetClinicID(sp.ClinicID).
		SetName(sp.Name).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrDuplicateSpecialty
		}
		return nil, fmt.Errorf("specialty repository: create: %w", err)
	}
	return toSpecialtyDomain(saved), nil
}

func (r *specialtyRepository) Delete(ctx context.Context, clinicID, id uuid.UUID) error {
	_, err := r.client.Specialty.Delete().
		Where(
			specialty.IDEQ(id),
			specialty.ClinicIDEQ(clinicID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("specialty repository: delete: %w", err)
	}
	return nil
}

func (r *specialtyRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]usermodel.Specialty, error) {
	u, err := r.client.User.Query().
		Where(user.IDEQ(userID)).
		WithSpecialties(func(sq *ent.SpecialtyQuery) {
			sq.Order(ent.Asc(specialty.FieldName))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, usermodel.ErrUserNotFound
		}
		return nil, fmt.Errorf("specialty repository: list by user: %w", err)
	}

	rows := make([]usermodel.Specialty, 0, len(u.Edges.Specialties))
	for _, sp := range u.Edges.Specialties {
		rows = append(rows, *toSpecialtyDomain(sp))
	}
	return rows, nil
}

func (r *specialtyRepository) CheckClinicScope(ctx context.Context, clinicID uuid.UUID, specialtyIDs []uuid.UUID) (bool, error) {
	count, err := r.client.Specialty.Query().
		Where(
			specialty.IDIn(specialtyIDs...),
			specialty.ClinicIDEQ(clinicID),
		).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("specialty repository: check clinic scope: %w", err)
	}
	return count == len(specialtyIDs), nil
}

func toSpecialtyDomain(sp *ent.Specialty) *usermodel.Specialty {
	if sp == nil {
		return nil
	}
	return &usermodel.Specialty{
		ID:        sp.ID,
		ClinicID:  sp.ClinicID,
		Name:      sp.Name,
		CreatedAt: sp.CreatedAt,
	}
}
