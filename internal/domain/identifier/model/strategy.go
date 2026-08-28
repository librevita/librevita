package model

import (
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

// RawSystem is the reserved URN of the built-in fallback strategy. It
// is never configurable: administrators register specific systems,
// everything else falls back to raw.
const RawSystem = "urn:librevita:id:raw"

// SystemURNPrefix is the namespace administrators use when registering
// a new document system.
const SystemURNPrefix = "urn:librevita:id:"

// Seed system URNs, the defaults shipped in the migration. They are
// rows, not a closed set: deployments in other jurisdictions register
// their own systems, and these can be edited or deactivated.
const (
	CPFSystem = "urn:librevita:id:br:cpf"
	SUSSystem = "urn:librevita:id:br:sus"
	NIFSystem = "urn:librevita:id:pt:nif"
	// #nosec G101 -- PassportSystem is a system URN (the FHIR system
	// of a document kind), not a credential; the name tripped the
	// hardcoded-secret heuristic.
	PassportSystem = "urn:librevita:id:passport"
)

// maxValueLen bounds every normalized value; the blind index hashes
// exactly what is stored, so unbounded input would unbounded the row.
const maxValueLen = 120

// Strategy validates and normalizes values of one document system.
type Strategy interface {
	// System returns the system URN, e.g. "urn:librevita:id:br:cpf".
	// It is the FHIR system of the stored identifier.
	System() string

	// Detect reports whether raw plausibly belongs to this system by
	// shape only, before normalization. It must be cheap and never
	// error.
	Detect(raw string) bool

	// Normalize canonicalizes raw to the stored and searchable form,
	// validating the shape and check digits where the system defines
	// them. It returns a ValidationError with a user-facing message
	// otherwise.
	Normalize(raw string) (string, error)
}

// rawStrategy is the fallback: it accepts any value, normalizing it to
// collapsed uppercase. It is always last in the detection order.
type rawStrategy struct{}

// System implements Strategy.
func (rawStrategy) System() string { return RawSystem }

// Detect implements Strategy.
func (rawStrategy) Detect(string) bool { return true }

// Normalize implements Strategy.
func (rawStrategy) Normalize(raw string) (string, error) {
	value := strings.ToUpper(collapseSpaces(strings.TrimSpace(raw)))
	if value == "" {
		return "", ErrValueRequired
	}
	if len(value) > maxValueLen {
		return "", &ValidationError{Msg: "value is too long"}
	}
	return value, nil
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

func compilePattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, errors.New("pattern is empty")
	}
	return regexp.Compile(`^(?:` + pattern + `)$`)
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isDigit(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func allEqual(s string) bool {
	for _, r := range s {
		if r != rune(s[0]) {
			return false
		}
	}
	return true
}
