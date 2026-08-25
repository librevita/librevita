package usecase

import (
	identifiermodel "librevita.org/internal/domain/identifier/model"
)

// Re-export identifier models, constants, and errors for consumers.
type (
	Transform            = identifiermodel.Transform
	CheckAlgorithm       = identifiermodel.CheckAlgorithm
	IdentifierSystem     = identifiermodel.IdentifierSystem
	IdentifierRecord     = identifiermodel.IdentifierRecord
	Identifier           = identifiermodel.Identifier
	SystemConfig         = identifiermodel.SystemConfig
	Strategy             = identifiermodel.Strategy
	Registry             = identifiermodel.Registry
	SystemRepository     = identifiermodel.SystemRepository
	IdentifierRepository = identifiermodel.IdentifierRepository
	ValidationError      = identifiermodel.ValidationError
)

const (
	TransformNone   = identifiermodel.TransformNone
	TransformDigits = identifiermodel.TransformDigits
	TransformUpper  = identifiermodel.TransformUpper
	TransformLower  = identifiermodel.TransformLower

	CheckNone        = identifiermodel.CheckNone
	CheckMod11Desc   = identifiermodel.CheckMod11Desc
	CheckMod11Cyclic = identifiermodel.CheckMod11Cyclic

	RawSystem      = identifiermodel.RawSystem
	CPFSystem      = identifiermodel.CPFSystem
	SUSSystem      = identifiermodel.SUSSystem
	NIFSystem      = identifiermodel.NIFSystem
	PassportSystem = identifiermodel.PassportSystem
)

var (
	ErrSystemNotFound      = identifiermodel.ErrSystemNotFound
	ErrDuplicate           = identifiermodel.ErrDuplicate
	ErrDuplicateIdentifier = identifiermodel.ErrDuplicateIdentifier
	ErrSystemInactive      = identifiermodel.ErrSystemInactive
	ErrSystemImmutable     = identifiermodel.ErrSystemImmutable
	ErrNotFound            = identifiermodel.ErrNotFound
	ErrValueRequired       = identifiermodel.ErrValueRequired
	ErrSystemNotAllowed    = identifiermodel.ErrSystemNotAllowed
)

// Input is a document as typed at reception.
type Input struct {
	PatientID string
	System    string
	Value     string
}

// SystemInput is the editable definition of a document system.
type SystemInput struct {
	System           string
	DisplayName      string
	Pattern          string
	Mask             string
	Transform        Transform
	CheckAlgorithm   CheckAlgorithm
	CheckBaseLen     int
	CheckDVCount     int
	CheckStartWeight int
}
