package usecase

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
)

const maxSpecialtyNameLen = 60

// ListSpecialties returns the clinic's specialties, alphabetically.
func (s *Service) ListSpecialties(ctx context.Context, clinicID string) ([]Specialty, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}
	return s.specialtyRepo.ListByClinic(ctx, cUUID)
}

// ListSpecialtiesPage returns one page of the clinic's specialties plus the total.
func (s *Service) ListSpecialtiesPage(ctx context.Context, clinicID string, limit, offset int) ([]Specialty, int64, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, 0, errors.Wrap(err, "usecase: invalid clinic id")
	}
	return s.specialtyRepo.ListPageByClinic(ctx, cUUID, limit, offset)
}

// CreateSpecialty adds a specialty to the clinic catalog.
func (s *Service) CreateSpecialty(ctx context.Context, clinicID, name string) (*Specialty, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "specialty name is required"}
	}
	if len(name) > maxSpecialtyNameLen {
		return nil, &ValidationError{Msg: "specialty name is too long"}
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.Wrap(err, "usecase: generate specialty id")
	}
	return s.specialtyRepo.Create(ctx, &Specialty{
		ID:       id,
		ClinicID: cUUID,
		Name:     name,
	})
}

// DeleteSpecialty removes a specialty from the clinic catalog.
func (s *Service) DeleteSpecialty(ctx context.Context, clinicID, id string) error {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid clinic id")
	}
	spUUID, err := uuid.Parse(id)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid specialty id")
	}
	return s.specialtyRepo.Delete(ctx, cUUID, spUUID)
}

// UserSpecialties returns the specialties assigned to a user account.
func (s *Service) UserSpecialties(ctx context.Context, userID string) ([]Specialty, error) {
	uUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid user id")
	}
	return s.specialtyRepo.ListByUser(ctx, uUUID)
}

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

// SetUserSpecialties replaces the account's specialty set in one transaction.
func (s *Service) SetUserSpecialties(ctx context.Context, clinicID, userID string, specialtyIDs []string) error {
	if err := validateSpecialtyIDs(specialtyIDs); err != nil {
		return err
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid clinic id")
	}
	uUUID, err := uuid.Parse(userID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid user id")
	}

	var validUUIDs []uuid.UUID
	for _, id := range specialtyIDs {
		if id == "" {
			continue
		}
		spUUID := uuid.MustParse(id)
		validUUIDs = append(validUUIDs, spUUID)
	}

	if len(validUUIDs) > 0 {
		ok, err := s.specialtyRepo.CheckClinicScope(ctx, cUUID, validUUIDs)
		if err != nil {
			return errors.Wrap(err, "usecase: check specialty scope")
		}
		if !ok {
			return ErrSpecialtyScope
		}
	}

	return s.userRepo.SetSpecialties(ctx, uUUID, validUUIDs)
}
