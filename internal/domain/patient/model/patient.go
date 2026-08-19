package model

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/types"
)

// Domain errors.
var (
	ErrNotFound  = errors.New("patient: not found")
	ErrForbidden = errors.New("patient: permission denied")
)

// PatientPayload is the encrypted PII/PHI payload stored inside encrypted_payload.
type PatientPayload struct {
	DisplayName string    `json:"display_name"`
	BirthDate   *string   `json:"birth_date,omitempty"`
	Sex         types.Sex `json:"sex"`
	Phone       *string   `json:"phone,omitempty"`
	Email       *string   `json:"email,omitempty"`
	Street      *string   `json:"street,omitempty"`
	City        *string   `json:"city,omitempty"`
	State       *string   `json:"state,omitempty"`
	PostalCode  *string   `json:"postal_code,omitempty"`
	Notes       *string   `json:"notes,omitempty"`
}

// Patient represents the decrypted in-memory patient domain model.
type Patient struct {
	ID          uuid.UUID
	ClinicID    uuid.UUID
	DisplayName string
	BirthDate   *string
	Sex         types.Sex
	Phone       *string
	Email       *string
	Street      *string
	City        *string
	State       *string
	PostalCode  *string
	Notes       *string
	Status      types.PatientStatus
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PatientRecord is the physical encrypted row stored in the database.
type PatientRecord struct {
	ID               uuid.UUID
	ClinicID         uuid.UUID
	BlindIndex       string
	EncryptedPayload []byte
	Nonce            []byte
	Status           types.PatientStatus
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PatientRecordWithCreator includes creator metadata for the patient record.
type PatientRecordWithCreator struct {
	Record       PatientRecord
	CreatorName  *string
	CreatorEmail *string
}

// GetPatientWithCreatorRow is a projection that includes creator metadata.
type GetPatientWithCreatorRow struct {
	ID           uuid.UUID
	ClinicID     uuid.UUID
	DisplayName  string
	BirthDate    *string
	Sex          types.Sex
	Phone        *string
	Email        *string
	Street       *string
	City         *string
	State        *string
	PostalCode   *string
	Notes        *string
	Status       types.PatientStatus
	CreatedBy    *uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatorEmail *string
	CreatorName  *string
}

// PatientInput is the editable profile of a patient.
type PatientInput struct {
	DisplayName string
	BirthDate   string
	Sex         types.Sex
	Phone       string
	Email       string
	Street      string
	City        string
	State       string
	PostalCode  string
	Notes       string

	IdentifierSystem string
	IdentifierValue  string
}

// PatientRepository defines the storage interface for patient records.
type PatientRepository interface {
	Create(ctx context.Context, rec PatientRecord) (*PatientRecord, error)
	Get(ctx context.Context, clinicID, patientID uuid.UUID) (*PatientRecord, error)
	GetWithCreator(ctx context.Context, clinicID, patientID uuid.UUID) (*PatientRecordWithCreator, error)
	Update(ctx context.Context, rec PatientRecord) (*PatientRecord, error)
	BulkSetStatus(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID, status types.PatientStatus) (int, error)
	ListByClinicAndStatus(ctx context.Context, clinicID uuid.UUID, status *types.PatientStatus) ([]PatientRecord, error)
	Count(ctx context.Context, clinicID uuid.UUID) (int, error)
}
