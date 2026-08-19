package identifier

import (
	"fmt"

	patientmodel "librevita.org/internal/domain/patient/model"
)

// Re-export identifier models, constants, and repository contracts from patient/model.
type (
	Transform            = patientmodel.Transform
	CheckAlgorithm       = patientmodel.CheckAlgorithm
	IdentifierSystem     = patientmodel.IdentifierSystem
	IdentifierRecord     = patientmodel.IdentifierRecord
	SystemRepository     = patientmodel.SystemRepository
	IdentifierRepository = patientmodel.IdentifierRepository
)

const (
	TransformNone   = patientmodel.TransformNone
	TransformDigits = patientmodel.TransformDigits
	TransformUpper  = patientmodel.TransformUpper
	TransformLower  = patientmodel.TransformLower

	CheckNone        = patientmodel.CheckNone
	CheckMod11Desc   = patientmodel.CheckMod11Desc
	CheckMod11Cyclic = patientmodel.CheckMod11Cyclic
)

// SystemConfig is the validated view of one identifier_systems row.
type SystemConfig struct {
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

// ParseSystemConfig converts a stored row to its validated form.
func ParseSystemConfig(row *IdentifierSystem) (SystemConfig, error) {
	cfg := SystemConfig{
		System:           row.System,
		DisplayName:      row.DisplayName,
		Pattern:          row.Pattern,
		Mask:             row.Mask,
		Transform:        row.Transform,
		CheckAlgorithm:   row.CheckAlgorithm,
		CheckBaseLen:     row.CheckBaseLen,
		CheckDVCount:     row.CheckDVCount,
		CheckStartWeight: row.CheckStartWeight,
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
	if len(c.Mask) > 64 {
		return fmt.Errorf("mask must be at most 64 characters")
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
