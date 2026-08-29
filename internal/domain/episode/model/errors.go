package model

import "github.com/cockroachdb/errors"

// Domain errors.
var (
	ErrNotFound       = errors.New("episode: not found")
	ErrForbidden      = errors.New("episode: permission denied")
	ErrNotDraft       = errors.New("episode: not a draft")
	ErrNotFinalized   = errors.New("episode: not finalized")
	ErrAlreadyAmended = errors.New("episode: already amended")
	ErrInvalidSOAP    = errors.New("episode: invalid SOAP document")
	ErrPatientGone    = errors.New("episode: patient not found")
)
