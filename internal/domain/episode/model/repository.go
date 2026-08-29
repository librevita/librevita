package model

import (
	"context"

	"github.com/google/uuid"
)

// EpisodeRepository is the persistence contract for the SOAP aggregate.
type EpisodeRepository interface {
	Create(ctx context.Context, ep Episode) (*Episode, error)
	UpdateDraft(ctx context.Context, ep Episode) (*Episode, error)
	Get(ctx context.Context, clinicID, episodeID uuid.UUID) (*Episode, error)
	ListByPatient(ctx context.Context, clinicID, patientID uuid.UUID, status *EpisodeStatus) ([]Episode, error)
	GetByPredecessor(ctx context.Context, clinicID, predecessorID uuid.UUID) (*Episode, error)
	SetStatus(ctx context.Context, clinicID, episodeID uuid.UUID, status EpisodeStatus) error
	PatientExists(ctx context.Context, clinicID, patientID uuid.UUID) (bool, error)
}
