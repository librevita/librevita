package model

import (
	"time"

	"librevita.org/pkg/ident"
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
	ID          ident.FindingID
	ClinicID    ident.ClinicID
	PatientID   ident.PatientID
	EpisodeID   ident.EpisodeID
	Code        Coding
	Value       FindingValue
	Status      FindingStatus
	EffectiveAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
