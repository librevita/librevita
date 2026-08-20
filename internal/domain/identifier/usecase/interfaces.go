package usecase

import (
	"context"

	identifiermodel "librevita.org/internal/domain/identifier/model"
)

// Service defines the contract for identifier encryption, validation, and querying.
type Service interface {
	AddIdentifier(ctx context.Context, clinicID, createdBy string, in Input) (*identifiermodel.Identifier, error)
	FindByValue(ctx context.Context, clinicID, raw string) ([]*identifiermodel.Identifier, error)
	List(ctx context.Context, clinicID, patientID string) ([]*identifiermodel.Identifier, error)
	ListByPatients(ctx context.Context, patientIDs []string) (map[string][]string, error)
	Remove(ctx context.Context, clinicID, patientID, identifierID string) error
	ValidateValue(system, raw string) (string, error)
}

// SystemsService defines the contract for administering document systems.
type SystemsService interface {
	List(ctx context.Context) ([]*identifiermodel.IdentifierSystem, error)
	SystemByID(ctx context.Context, id string) (*identifiermodel.IdentifierSystem, error)
	Create(ctx context.Context, createdBy string, in SystemInput) (*identifiermodel.IdentifierSystem, error)
	Update(ctx context.Context, id string, in SystemInput) (*identifiermodel.IdentifierSystem, error)
	SetActive(ctx context.Context, id string, active bool) error
}
