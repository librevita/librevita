package model

import (
	"time"

	"github.com/google/uuid"
)

// Quantity is a measured amount with optional UCUM coding.
type Quantity struct {
	Value  float64
	Unit   string
	Code   string
	System string
}

// FindingValue is the typed payload of one objective finding.
type FindingValue struct {
	Kind     FindingValueKind
	Quantity *Quantity
	String   string
	Boolean  *bool
	Coded    *Coding
}

// Finding is a structured objective finding belonging to an Episode.
type Finding struct {
	ID          uuid.UUID
	ClinicID    uuid.UUID
	PatientID   uuid.UUID
	EpisodeID   uuid.UUID
	Code        Coding
	Value       FindingValue
	Status      FindingStatus
	EffectiveAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
