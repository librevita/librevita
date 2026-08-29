package model

import (
	"time"

	"github.com/google/uuid"
)

// Problem is a structured assessment diagnosis belonging to an Episode.
type Problem struct {
	ID                 uuid.UUID
	ClinicID           uuid.UUID
	PatientID          uuid.UUID
	EpisodeID          uuid.UUID
	Code               Coding
	Text               string
	ClinicalStatus     ProblemClinicalStatus
	VerificationStatus ProblemVerificationStatus
	Category           ProblemCategory
	Rank               int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
