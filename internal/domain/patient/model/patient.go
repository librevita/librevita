package model

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
)

// Sex is the patient sex value. It mirrors the CHECK constraint on
// patients.sex (see db/migrations/00006_patients.sql).
type Sex string

const (
	SexFemale  Sex = "female"
	SexMale    Sex = "male"
	SexOther   Sex = "other"
	SexUnknown Sex = "unknown"
)

// Valid reports whether s is one of the options the database CHECK
// constraint accepts.
func (s Sex) Valid() bool {
	switch s {
	case SexFemale, SexMale, SexOther, SexUnknown:
		return true
	}
	return false
}

// String returns the stored representation of s.
func (s Sex) String() string {
	return string(s)
}

// ParseSex converts a stored value back to the enum. ok is false when
// the value is not one of the CHECK options.
func ParseSex(s string) (Sex, bool) {
	sex := Sex(s)
	return sex, sex.Valid()
}

// PatientStatus is the patient record status. It mirrors the CHECK
// constraint on patients.status (see db/migrations/00006_patients.sql).
type PatientStatus string

const (
	PatientStatusActive   PatientStatus = "active"
	PatientStatusInactive PatientStatus = "inactive"
	PatientStatusArchived PatientStatus = "archived"
)

// Valid reports whether s is one of the options the database CHECK
// constraint accepts.
func (s PatientStatus) Valid() bool {
	switch s {
	case PatientStatusActive, PatientStatusInactive, PatientStatusArchived:
		return true
	}
	return false
}

// String returns the stored representation of s.
func (s PatientStatus) String() string {
	return string(s)
}

// ParsePatientStatus converts a stored value back to the enum. ok is
// false when the value is not one of the CHECK options.
func ParsePatientStatus(s string) (PatientStatus, bool) {
	status := PatientStatus(s)
	return status, status.Valid()
}

// Domain errors.
var (
	ErrNotFound  = errors.New("patient: not found")
	ErrForbidden = errors.New("patient: permission denied")
)

// Patient represents the in-memory patient domain model.
type Patient struct {
	ID          uuid.UUID
	ClinicID    uuid.UUID
	DisplayName string
	BirthDate   *string
	Sex         Sex
	Phone       *string
	Email       *string
	Street      *string
	City        *string
	State       *string
	PostalCode  *string
	Notes       *string
	Status      PatientStatus
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GetPatientWithCreatorRow is a projection that includes creator metadata.
type GetPatientWithCreatorRow struct {
	ID           uuid.UUID
	ClinicID     uuid.UUID
	DisplayName  string
	BirthDate    *string
	Sex          Sex
	Phone        *string
	Email        *string
	Street       *string
	City         *string
	State        *string
	PostalCode   *string
	Notes        *string
	Status       PatientStatus
	CreatedBy    *uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatorEmail *string
	CreatorName  *string
}

// PatientCandidate contains only the non-sensitive fields needed to select a
// page before loading patient PHI and Patient DEKs.
type PatientCandidate struct {
	ID        uuid.UUID
	ClinicID  uuid.UUID
	Status    PatientStatus
	CreatedAt time.Time
}

// PatientInput is the editable profile of a patient.
type PatientInput struct {
	DisplayName string
	BirthDate   string
	Sex         Sex
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

// PatientRepository defines the storage interface for patient domain models with transparent encryption.
type PatientRepository interface {
	Create(ctx context.Context, patient Patient) (*Patient, error)
	Get(ctx context.Context, clinicID, patientID uuid.UUID) (*Patient, error)
	GetWithCreator(ctx context.Context, clinicID, patientID uuid.UUID) (*GetPatientWithCreatorRow, error)
	Update(ctx context.Context, patient Patient) (*Patient, error)
	BulkSetStatus(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID, status PatientStatus) (int, error)
	ListByClinicAndStatus(ctx context.Context, clinicID uuid.UUID, status *PatientStatus) ([]Patient, error)
	Count(ctx context.Context, clinicID uuid.UUID) (int, error)
}

// PatientDeletionRepository performs an idempotent aggregate delete for one
// clinic patient and all relational clinical rows owned by it.
type PatientDeletionRepository interface {
	PatientRepository
	DeleteAggregate(ctx context.Context, clinicID, patientID uuid.UUID) error
}

// PatientQueryRepository is the optimized extension used by the production
// repository. Candidate selection happens in SQL and PHI hydration is
// limited to the returned page.
type PatientQueryRepository interface {
	PatientRepository
	ListCandidates(ctx context.Context, clinicID uuid.UUID, status *PatientStatus, nameTokens []string, emailBlindIndex string, limit, offset int) ([]PatientCandidate, int, error)
	GetMany(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID) ([]Patient, error)
}
