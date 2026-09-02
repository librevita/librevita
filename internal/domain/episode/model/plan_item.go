package model

import (
	"time"

	"librevita.org/pkg/ident"
)

// PlanItem is a structured plan activity belonging to an Episode.
type PlanItem struct {
	ID          ident.PlanItemID
	ClinicID    ident.ClinicID
	PatientID   ident.PatientID
	EpisodeID   ident.EpisodeID
	Kind        PlanItemKind
	Code        Coding
	Description string
	Status      PlanItemStatus
	ScheduledAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
