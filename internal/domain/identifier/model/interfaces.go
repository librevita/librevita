package model

import (
	"context"

	"librevita.org/pkg/ident"
)

// SystemRepository defines the persistence contract for document systems.
type SystemRepository interface {
	ListActive(ctx context.Context) ([]*IdentifierSystem, error)
	ListAll(ctx context.Context) ([]*IdentifierSystem, error)
	GetByID(ctx context.Context, id ident.IdentifierSystemID) (*IdentifierSystem, error)
	GetBySystem(ctx context.Context, system string) (*IdentifierSystem, error)
	Create(ctx context.Context, sys *IdentifierSystem) (*IdentifierSystem, error)
	Update(ctx context.Context, sys *IdentifierSystem) (*IdentifierSystem, error)
	SetActive(ctx context.Context, id ident.IdentifierSystemID, active bool) error
	SeedDefaults(ctx context.Context) error
}

// IdentifierRepository defines the persistence contract for identifiers.
type IdentifierRepository interface {
	Add(ctx context.Context, rec IdentifierRecord) (*IdentifierRecord, error)
	AllowsSystem(ctx context.Context, clinicID ident.ClinicID, system string) (bool, error)
	FindByBlindIndex(ctx context.Context, clinicID ident.ClinicID, blindIndex string) (*IdentifierRecord, error)
	ListByPatient(ctx context.Context, patientID ident.PatientID) ([]IdentifierRecord, error)
	ListByPatients(ctx context.Context, patientIDs []ident.PatientID) ([]IdentifierRecord, error)
	Remove(ctx context.Context, patientID ident.PatientID, identifierID ident.PatientIdentifierID) error
	PatientExists(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (bool, error)
}
