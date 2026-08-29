package model

import (
	"time"

	"github.com/google/uuid"
)

// PlanItem is a structured plan activity belonging to an Episode.
type PlanItem struct {
	ID          uuid.UUID
	ClinicID    uuid.UUID
	PatientID   uuid.UUID
	EpisodeID   uuid.UUID
	Kind        PlanItemKind
	Code        Coding
	Description string
	Status      PlanItemStatus
	ScheduledAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
