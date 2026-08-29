package model

import (
	"time"

	"github.com/google/uuid"
)

// SOAP is the four narrative sections of a clinical note.
type SOAP struct {
	Subjective string
	Objective  string
	Assessment string
	Plan       string
}

// Episode is the SOAP chart aggregate for one encounter.
type Episode struct {
	ID            uuid.UUID
	ClinicID      uuid.UUID
	PatientID     uuid.UUID
	AuthorID      uuid.UUID
	AppointmentID *uuid.UUID
	PredecessorID *uuid.UUID
	SuccessorID   *uuid.UUID
	Type          EpisodeType
	Status        EpisodeStatus
	Class         CareSetting
	OccurredAt    time.Time
	EndedAt       *time.Time
	SOAP          SOAP
	Findings      []Finding
	Problems      []Problem
	PlanItems     []PlanItem
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CanAmend reports whether this finalized note has no successor yet.
func (e Episode) CanAmend() bool {
	return e.Status == EpisodeStatusFinalized && e.SuccessorID == nil
}

// Validate reports structural problems on the aggregate (enums, ranks).
func (e Episode) Validate() error {
	if e.ClinicID == uuid.Nil || e.PatientID == uuid.Nil || e.AuthorID == uuid.Nil {
		return ErrInvalidSOAP
	}
	if !e.Type.Valid() || !e.Status.Valid() || !e.Class.Valid() {
		return ErrInvalidSOAP
	}
	if e.OccurredAt.IsZero() {
		return ErrInvalidSOAP
	}
	for _, f := range e.Findings {
		if !f.Status.Valid() || !f.Value.Kind.Valid() || f.Code.Empty() {
			return ErrInvalidSOAP
		}
	}
	for _, p := range e.Problems {
		if !p.ClinicalStatus.Valid() || !p.VerificationStatus.Valid() || !p.Category.Valid() {
			return ErrInvalidSOAP
		}
		if p.Rank < 1 {
			return ErrInvalidSOAP
		}
	}
	for _, item := range e.PlanItems {
		if !item.Kind.Valid() || !item.Status.Valid() {
			return ErrInvalidSOAP
		}
	}
	return nil
}
