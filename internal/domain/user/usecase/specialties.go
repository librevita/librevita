package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/domain/user/repository"
)

// Specialty errors.
var ErrDuplicateSpecialty = errors.New("usecase: specialty already exists")

// ErrSpecialtyScope reports a specialty that does not belong to the
// request's clinic.
var ErrSpecialtyScope = errors.New("usecase: specialty does not belong to this clinic")

const maxSpecialtyNameLen = 60

// ListSpecialties returns the clinic's specialties, alphabetically.
func (s *Service) ListSpecialties(ctx context.Context, clinicID string) ([]repository.Specialty, error) {
	rows, err := s.users.ListSpecialties(ctx, uuid.MustParse(clinicID))
	if err != nil {
		return nil, fmt.Errorf("usecase: list specialties: %w", err)
	}
	return rows, nil
}

// ListSpecialtiesPage returns one page of the clinic's specialties plus
// the total.
func (s *Service) ListSpecialtiesPage(ctx context.Context, clinicID string, limit, offset int) ([]repository.Specialty, int64, error) {
	rows, err := s.users.ListSpecialtiesPage(ctx, repository.ListSpecialtiesPageParams{
		ClinicID: uuid.MustParse(clinicID), Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list specialties page: %w", err)
	}
	total, err := s.users.CountSpecialties(ctx, uuid.MustParse(clinicID))
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: count specialties: %w", err)
	}
	return rows, total, nil
}

// CreateSpecialty adds a specialty to the clinic catalog.
func (s *Service) CreateSpecialty(ctx context.Context, clinicID, name string) (*repository.Specialty, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "specialty name is required"}
	}
	if len(name) > maxSpecialtyNameLen {
		return nil, &ValidationError{Msg: "specialty name is too long"}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate specialty id: %w", err)
	}
	specialty, err := s.users.CreateSpecialty(ctx, repository.CreateSpecialtyParams{
		ID: id, ClinicID: uuid.MustParse(clinicID), Name: name,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicateSpecialty
		}
		return nil, fmt.Errorf("usecase: create specialty: %w", err)
	}
	return &specialty, nil
}

// DeleteSpecialty removes a specialty from the clinic catalog. The
// cascade drops the user mappings.
func (s *Service) DeleteSpecialty(ctx context.Context, clinicID, id string) error {
	if err := s.users.DeleteSpecialty(ctx, repository.DeleteSpecialtyParams{
		ID: uuid.MustParse(id), ClinicID: uuid.MustParse(clinicID),
	}); err != nil {
		return fmt.Errorf("usecase: delete specialty: %w", err)
	}
	return nil
}

// UserSpecialties returns the specialties assigned to a user account.
func (s *Service) UserSpecialties(ctx context.Context, userID string) ([]repository.Specialty, error) {
	rows, err := s.users.ListUserSpecialties(ctx, uuid.MustParse(userID))
	if err != nil {
		return nil, fmt.Errorf("usecase: list user specialties: %w", err)
	}
	return rows, nil
}

// validateSpecialtyIDs rejects ids that are not UUIDs, so malformed
// input fails with a validation error instead of panicking later in
// uuid.MustParse (SetUserSpecialties, and the stored staff change JSON
// on approval).
func validateSpecialtyIDs(ids []string) error {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return &ValidationError{Msg: "invalid specialty id"}
		}
	}
	return nil
}

// SetUserSpecialties replaces the account's specialty set in one
// transaction. Every specialty must belong to the request's clinic:
// the UI only lists the clinic's catalog, but the write path enforces
// the scope too, so a specialty id of another clinic is rejected
// instead of silently cross-linking accounts.
func (s *Service) SetUserSpecialties(ctx context.Context, clinicID, userID string, specialtyIDs []string) error {
	if err := validateSpecialtyIDs(specialtyIDs); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usecase: begin specialty tx: %w", err)
	}
	queries := repository.New(tx)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, id := range specialtyIDs {
		if id == "" {
			continue
		}
		if _, err := queries.GetSpecialtyByID(ctx, repository.GetSpecialtyByIDParams{
			ID: uuid.MustParse(id), ClinicID: uuid.MustParse(clinicID),
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSpecialtyScope
			}
			return fmt.Errorf("usecase: check specialty scope: %w", err)
		}
	}
	if err := queries.ClearUserSpecialties(ctx, uuid.MustParse(userID)); err != nil {
		return fmt.Errorf("usecase: clear user specialties: %w", err)
	}
	for _, id := range specialtyIDs {
		if id == "" {
			continue
		}
		if err := queries.AddUserSpecialty(ctx, repository.AddUserSpecialtyParams{
			UserID: uuid.MustParse(userID), SpecialtyID: uuid.MustParse(id),
		}); err != nil {
			return fmt.Errorf("usecase: add user specialty: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("usecase: commit specialty tx: %w", err)
	}
	committed = true
	return nil
}
