package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/domain/user/repository"
)

// Specialty errors.
var ErrDuplicateSpecialty = errors.New("usecase: specialty already exists")

const maxSpecialtyNameLen = 60

// ListSpecialties returns the clinic's specialties, alphabetically.
func (s *Service) ListSpecialties(ctx context.Context, clinicID string) ([]repository.Specialty, error) {
	rows, err := s.users.ListSpecialties(ctx, clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: list specialties: %w", err)
	}
	return rows, nil
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
		ID: id.String(), ClinicID: clinicID, Name: name,
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
		ID: id, ClinicID: clinicID,
	}); err != nil {
		return fmt.Errorf("usecase: delete specialty: %w", err)
	}
	return nil
}

// UserSpecialties returns the specialties assigned to a user account.
func (s *Service) UserSpecialties(ctx context.Context, userID string) ([]repository.Specialty, error) {
	rows, err := s.users.ListUserSpecialties(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("usecase: list user specialties: %w", err)
	}
	return rows, nil
}

// SetUserSpecialties replaces the account's specialty set in one
// transaction.
func (s *Service) SetUserSpecialties(ctx context.Context, userID string, specialtyIDs []string) error {
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
	if err := queries.ClearUserSpecialties(ctx, userID); err != nil {
		return fmt.Errorf("usecase: clear user specialties: %w", err)
	}
	for _, id := range specialtyIDs {
		if id == "" {
			continue
		}
		if err := queries.AddUserSpecialty(ctx, repository.AddUserSpecialtyParams{
			UserID: userID, SpecialtyID: id,
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
