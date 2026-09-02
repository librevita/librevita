package ident

//go:generate go run -mod=mod gen.go

import "github.com/google/uuid"

// ClinicID is the primary key of a Clinic.
type ClinicID uuid.UUID

// UserID is the primary key of a clinic User.
type UserID uuid.UUID

// PlatformUserID is the primary key of a PlatformUser (apex operator).
type PlatformUserID uuid.UUID

// PatientID is the primary key of a Patient.
type PatientID uuid.UUID

// EpisodeID is the primary key of an Episode.
type EpisodeID uuid.UUID

// AppointmentID is the primary key of an Appointment.
type AppointmentID uuid.UUID

// FindingID is the primary key of a Finding.
type FindingID uuid.UUID

// ProblemID is the primary key of a Problem.
type ProblemID uuid.UUID

// PlanItemID is the primary key of a PlanItem.
type PlanItemID uuid.UUID

// RoleID is the primary key of a Role.
type RoleID uuid.UUID

// SpecialtyID is the primary key of a Specialty.
type SpecialtyID uuid.UUID

// PolicyID is the primary key of an AccessPolicy.
type PolicyID uuid.UUID

// PatientIdentifierID is the primary key of a PatientIdentifier row.
type PatientIdentifierID uuid.UUID

// IdentifierSystemID is the primary key of an IdentifierSystem.
type IdentifierSystemID uuid.UUID

// ClinicIdentifierSystemID is the primary key of a ClinicIdentifierSystem opt-in.
type ClinicIdentifierSystemID uuid.UUID

// StorageObjectID is the primary key of a StorageObject.
type StorageObjectID uuid.UUID

// StaffChangeRequestID is the primary key of a StaffChangeRequest.
type StaffChangeRequestID uuid.UUID
