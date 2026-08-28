package model

import "github.com/cockroachdb/errors"

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
	if err := cfg.ValidateShape(); err != nil {
		return SystemConfig{}, err
	}
	if _, err := compilePattern(cfg.Pattern); err != nil {
		return SystemConfig{}, errors.Wrapf(err, "pattern %q", cfg.Pattern)
	}
	return cfg, nil
}

// ValidateShape checks the cross-field invariants without touching the
// pattern.
func (c SystemConfig) ValidateShape() error {
	if len(c.System) < 3 || len(c.System) > 64 {
		return errors.New("system must be between 3 and 64 characters")
	}
	if c.System == RawSystem {
		return errors.Newf("system %q is reserved", errors.Safe(RawSystem))
	}
	if c.DisplayName == "" {
		return errors.New("display name is required")
	}
	if len(c.Mask) > 64 {
		return errors.New("mask must be at most 64 characters")
	}
	if !c.Transform.Valid() {
		return errors.Newf("invalid transform %q", errors.Safe(string(c.Transform)))
	}
	if !c.CheckAlgorithm.Valid() {
		return errors.Newf("invalid check algorithm %q", errors.Safe(string(c.CheckAlgorithm)))
	}
	if c.CheckAlgorithm == CheckNone {
		if c.CheckBaseLen != 0 {
			return errors.New("check base length must be 0 when no check algorithm is set")
		}
		return nil
	}
	if c.CheckBaseLen < 1 {
		return errors.New("check base length must be >= 1 when a check algorithm is set")
	}
	if c.CheckAlgorithm == CheckMod11Cyclic && c.CheckDVCount != 1 {
		return errors.New("mod11_cyclic supports exactly one check digit")
	}
	if c.CheckDVCount != 1 && c.CheckDVCount != 2 {
		return errors.New("check digit count must be 1 or 2")
	}
	if c.CheckStartWeight < 2 {
		return errors.New("check start weight must be >= 2")
	}
	return nil
}
