package model

import (
	"time"

	"librevita.org/pkg/ident"
)

// Problem is a structured assessment diagnosis belonging to an Episode.
type Problem struct {
	ID                 ident.ProblemID
	ClinicID           ident.ClinicID
	PatientID          ident.PatientID
	EpisodeID          ident.EpisodeID
	Code               Coding
	Text               string
	ClinicalStatus     ProblemClinicalStatus
	VerificationStatus ProblemVerificationStatus
	Category           ProblemCategory
	Rank               int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
