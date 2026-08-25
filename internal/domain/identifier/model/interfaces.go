package model

import (
	"context"

	"github.com/google/uuid"
)

// SystemRepository defines the persistence contract for document systems.
type SystemRepository interface {
	ListActive(ctx context.Context) ([]*IdentifierSystem, error)
	ListAll(ctx context.Context) ([]*IdentifierSystem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*IdentifierSystem, error)
	GetBySystem(ctx context.Context, system string) (*IdentifierSystem, error)
	Create(ctx context.Context, sys *IdentifierSystem) (*IdentifierSystem, error)
	Update(ctx context.Context, sys *IdentifierSystem) (*IdentifierSystem, error)
	SetActive(ctx context.Context, id uuid.UUID, active bool) error
	SeedDefaults(ctx context.Context) error
}

// IdentifierRepository defines the persistence contract for identifiers.
type IdentifierRepository interface {
	Add(ctx context.Context, rec IdentifierRecord) (*IdentifierRecord, error)
	AllowsSystem(ctx context.Context, clinicID uuid.UUID, system string) (bool, error)
	FindByBlindIndex(ctx context.Context, clinicID uuid.UUID, blindIndex string) (*IdentifierRecord, error)
	ListByPatient(ctx context.Context, patientID uuid.UUID) ([]IdentifierRecord, error)
	ListByPatients(ctx context.Context, patientIDs []uuid.UUID) ([]IdentifierRecord, error)
	Remove(ctx context.Context, patientID, identifierID uuid.UUID) error
	PatientExists(ctx context.Context, clinicID, patientID uuid.UUID) (bool, error)
}
