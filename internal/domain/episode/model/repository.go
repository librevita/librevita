package model

import (
	"context"

	"librevita.org/pkg/ident"
)

// EpisodeRepository is the persistence contract for the SOAP aggregate.
type EpisodeRepository interface {
	Create(ctx context.Context, ep Episode) (*Episode, error)
	UpdateDraft(ctx context.Context, ep Episode) (*Episode, error)
	Get(ctx context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID) (*Episode, error)
	ListByPatient(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID, status *EpisodeStatus) ([]Episode, error)
	GetByPredecessor(ctx context.Context, clinicID ident.ClinicID, predecessorID ident.EpisodeID) (*Episode, error)
	SetStatus(ctx context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID, status EpisodeStatus) error
	PatientExists(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (bool, error)
}
