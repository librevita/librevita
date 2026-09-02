package model

import (
	"time"

	"librevita.org/pkg/ident"
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
	ID            ident.EpisodeID
	ClinicID      ident.ClinicID
	PatientID     ident.PatientID
	AuthorID      ident.UserID
	AppointmentID *ident.AppointmentID
	PredecessorID *ident.EpisodeID
	SuccessorID   *ident.EpisodeID
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
	if err := e.validateHeader(); err != nil {
		return err
	}
	for _, f := range e.Findings {
		if err := validateFinding(f); err != nil {
			return err
		}
	}
	for _, p := range e.Problems {
		if err := validateProblem(p); err != nil {
			return err
		}
	}
	for _, item := range e.PlanItems {
		if err := validatePlanItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (e Episode) validateHeader() error {
	if e.ClinicID.IsZero() || e.PatientID.IsZero() || e.AuthorID.IsZero() {
		return ErrInvalidSOAP
	}
	if !e.Type.Valid() || !e.Status.Valid() || !e.Class.Valid() {
		return ErrInvalidSOAP
	}
	if e.OccurredAt.IsZero() {
		return ErrInvalidSOAP
	}
	return nil
}

func validateFinding(f Finding) error {
	if !f.Status.Valid() || !f.Value.Kind.Valid() || f.Code.Empty() {
		return ErrInvalidSOAP
	}
	return nil
}

func validateProblem(p Problem) error {
	if !p.ClinicalStatus.Valid() || !p.VerificationStatus.Valid() || !p.Category.Valid() {
		return ErrInvalidSOAP
	}
	if p.Rank < 1 {
		return ErrInvalidSOAP
	}
	return nil
}

func validatePlanItem(item PlanItem) error {
	if !item.Kind.Valid() || !item.Status.Valid() {
		return ErrInvalidSOAP
	}
	return nil
}
