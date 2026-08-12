package identifier

import (
	"fmt"

	"librevita.org/internal/domain/patient/repository"
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

// SystemConfig is the validated view of one identifier_systems row.
type SystemConfig struct {
	System           string
	DisplayName      string
	Pattern          string
	Transform        Transform
	CheckAlgorithm   CheckAlgorithm
	CheckBaseLen     int
	CheckDVCount     int
	CheckStartWeight int
}

// ParseSystemConfig converts a stored row to its validated form. It
// fails when the stored values cannot build a working strategy, which
// must never happen through SystemsService but guards against hand
// edited rows.
func ParseSystemConfig(row repository.IdentifierSystem) (SystemConfig, error) {
	cfg := SystemConfig{
		System:           row.System,
		DisplayName:      row.DisplayName,
		Pattern:          row.Pattern,
		Transform:        Transform(row.Transform),
		CheckAlgorithm:   CheckAlgorithm(row.CheckAlgorithm),
		CheckBaseLen:     int(row.CheckBaseLen),
		CheckDVCount:     int(row.CheckDvCount),
		CheckStartWeight: int(row.CheckStartWeight),
	}
	if err := cfg.validateShape(); err != nil {
		return SystemConfig{}, err
	}
	if _, err := compilePattern(cfg.Pattern); err != nil {
		return SystemConfig{}, fmt.Errorf("pattern %q: %w", cfg.Pattern, err)
	}
	return cfg, nil
}

// validateShape checks the cross-field invariants without touching the
// pattern.
func (c SystemConfig) validateShape() error {
	if len(c.System) < 3 || len(c.System) > 64 {
		return fmt.Errorf("system must be between 3 and 64 characters")
	}
	if c.System == RawSystem {
		return fmt.Errorf("system %q is reserved", RawSystem)
	}
	if c.DisplayName == "" {
		return fmt.Errorf("display name is required")
	}
	if !c.Transform.Valid() {
		return fmt.Errorf("invalid transform %q", c.Transform)
	}
	if !c.CheckAlgorithm.Valid() {
		return fmt.Errorf("invalid check algorithm %q", c.CheckAlgorithm)
	}
	if c.CheckAlgorithm == CheckNone {
		if c.CheckBaseLen != 0 {
			return fmt.Errorf("check base length must be 0 when no check algorithm is set")
		}
		return nil
	}
	if c.CheckBaseLen < 1 {
		return fmt.Errorf("check base length must be >= 1 when a check algorithm is set")
	}
	if c.CheckAlgorithm == CheckMod11Cyclic && c.CheckDVCount != 1 {
		return fmt.Errorf("mod11_cyclic supports exactly one check digit")
	}
	if c.CheckDVCount != 1 && c.CheckDVCount != 2 {
		return fmt.Errorf("check digit count must be 1 or 2")
	}
	if c.CheckStartWeight < 2 {
		return fmt.Errorf("check start weight must be >= 2")
	}
	return nil
}

// checkMod11Descending computes the modulo-11 check digit with weights
// start, start-1, ..., 2 over base. Residues 0 and 1 map to 0.
func checkMod11Descending(base string, start int) byte {
	sum := 0
	weight := start
	for _, r := range base {
		sum += int(r-'0') * weight
		weight--
	}
	rest := sum % 11
	if rest < 2 {
		return '0'
	}
	return byte('0' + (11 - rest))
}

// checkMod11Cyclic computes the modulo-11 check digit with weights 2..9
// cycling right-to-left over base. Check digits 10 and 11 map to 0.
func checkMod11Cyclic(base string) byte {
	sum := 0
	weight := 2
	for i := len(base) - 1; i >= 0; i-- {
		sum += int(base[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	dv := 11 - (sum % 11)
	if dv >= 10 {
		dv = 0
	}
	return byte('0' + dv)
}
