package model

import (
	"time"

	"github.com/google/uuid"
)

// Transform is the canonicalization mode applied to raw input before
// the pattern match and before encryption/indexing.
type Transform string

const (
	// TransformNone trims and collapses internal whitespace, keeping
	// the case as typed.
	TransformNone Transform = "none"
	// TransformDigits keeps only the digits, stripping punctuation and
	// letters (the usual mode for numeric documents such as CPF).
	TransformDigits Transform = "digits"
	// TransformUpper trims, collapses, and uppercases (passports and
	// alphanumeric numbers that are case-insensitive).
	TransformUpper Transform = "upper"
	// TransformLower trims, collapses, and lowercases.
	TransformLower Transform = "lower"
)

// Valid reports whether t is one of the transform modes the database
// CHECK constraint accepts.
func (t Transform) Valid() bool {
	switch t {
	case TransformNone, TransformDigits, TransformUpper, TransformLower:
		return true
	}
	return false
}

// CheckAlgorithm is the check-digit scheme of a document system.
type CheckAlgorithm string

const (
	// CheckNone disables check-digit validation.
	CheckNone CheckAlgorithm = "none"
	// CheckMod11Desc is the modulo-11 scheme with descending weights
	// (10..2 over the base digits, CPF and NIF style). Residues 0 and
	// 1 map to check digit 0.
	CheckMod11Desc CheckAlgorithm = "mod11_desc"
	// CheckMod11Cyclic is the modulo-11 scheme with weights 2..9
	// cycling right-to-left over the base digits (SUS card style).
	// Check digits 10 and 11 map to 0.
	CheckMod11Cyclic CheckAlgorithm = "mod11_cyclic"
)

// Valid reports whether a is one of the algorithms the database CHECK
// constraint accepts.
func (a CheckAlgorithm) Valid() bool {
	switch a {
	case CheckNone, CheckMod11Desc, CheckMod11Cyclic:
		return true
	}
	return false
}

// IdentifierSystem is the domain model representing a document system.
type IdentifierSystem struct {
	ID               uuid.UUID
	System           string
	DisplayName      string
	Pattern          string
	Mask             string
	Transform        Transform
	CheckAlgorithm   CheckAlgorithm
	CheckBaseLen     int
	CheckDVCount     int
	CheckStartWeight int
	Active           bool
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IdentifierRecord is the encrypted physical record of an identifier.
type IdentifierRecord struct {
	ID              uuid.UUID
	ClinicID        uuid.UUID
	PatientID       uuid.UUID
	System          string
	ValueCiphertext []byte
	Nonce           []byte
	BlindIndex      string
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Identifier represents a decrypted identification document.
type Identifier struct {
	ID        string
	PatientID string
	System    string
	Value     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}
