// Package identifier implements FHIR-style patient identification
// documents (Identifier: system + value). The value is stored
// encrypted at field level, and exact lookups use a keyed blind index,
// so the plaintext never reaches the database.
//
// Document systems are rows in identifier_systems, administered at
// runtime (see SystemsService): a deployment in any jurisdiction
// registers the shape and check-digit rules of its documents without
// shipping code. A Strategy wraps one configured system, or the
// built-in raw fallback, and validates and normalizes values; the
// stored, encrypted, and indexed form is always the normalized value.
package identifier

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"librevita.org/internal/domain/patient/repository"
)

// RawSystem is the reserved URN of the built-in fallback strategy. It
// is never configurable: administrators register specific systems,
// everything else falls back to raw.
const RawSystem = "urn:librevita:id:raw"

// systemURNPrefix is the namespace administrators use when registering
// a new document system.
const systemURNPrefix = "urn:librevita:id:"

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

// ValidationError is returned for values that do not fit the system's
// scheme (bad check digit, wrong length, ...).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ErrValueRequired reports an empty value.
var ErrValueRequired = errors.New("identifier: value is required")

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

// Registry is the runtime view of identifier_systems: a thread-safe
// cache of the active system configurations. SystemsService reloads it
// after every change, so a new document type is usable immediately
// without restarting the application.
type Registry struct {
	mu      sync.RWMutex
	systems map[string]*configured
	order   []string
}

// NewRegistry builds an empty registry. It contains no systems until
// Reload loads them from the database.
func NewRegistry() *Registry {
	return &Registry{systems: map[string]*configured{}}
}

// Reload replaces the cached system list from the database. It is
// called at boot and after every administrative change. Detection
// order is deterministic regardless of the query's own ordering:
// longest pattern first, then by URN.
func (r *Registry) Reload(systems []repository.IdentifierSystem) error {
	sorted := append([]repository.IdentifierSystem(nil), systems...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if len(sorted[i].Pattern) != len(sorted[j].Pattern) {
			return len(sorted[i].Pattern) > len(sorted[j].Pattern)
		}
		return sorted[i].System < sorted[j].System
	})

	next := make(map[string]*configured, len(sorted))
	order := make([]string, 0, len(sorted))
	for _, row := range sorted {
		c, err := newConfigured(row)
		if err != nil {
			return fmt.Errorf("identifier: system %q: %w", row.System, err)
		}
		next[row.System] = c
		order = append(order, row.System)
	}
	r.mu.Lock()
	r.systems = next
	r.order = order
	r.mu.Unlock()
	return nil
}

// ForSystem returns the strategy for a system URN, falling back to the
// raw strategy for unknown or deactivated systems.
func (r *Registry) ForSystem(system string) Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.systems[system]; ok {
		return s
	}
	return rawStrategy{}
}

// Detect returns the first active strategy whose shape matches raw,
// in the registry order (longest pattern first). The raw fallback
// matches everything, so it never returns nil.
func (r *Registry) Detect(raw string) Strategy {
	r.mu.RLock()
	order := r.order
	r.mu.RUnlock()
	for _, system := range order {
		r.mu.RLock()
		s := r.systems[system]
		r.mu.RUnlock()
		if s.Detect(raw) {
			return s
		}
	}
	return rawStrategy{}
}

// DetectCandidates returns the active systems whose shape matches raw,
// most specific first, plus the raw fallback. Callers derive one blind
// index per candidate and stop at the first hit; values that fail
// normalization are skipped.
func (r *Registry) DetectCandidates(raw string) []Strategy {
	r.mu.RLock()
	order := r.order
	r.mu.RUnlock()

	var candidates []Strategy
	for _, system := range order {
		r.mu.RLock()
		s := r.systems[system]
		r.mu.RUnlock()
		if s != nil && s.Detect(raw) {
			candidates = append(candidates, s)
		}
	}
	candidates = append(candidates, rawStrategy{})
	return candidates
}

// Systems returns the active system URNs in detection order.
func (r *Registry) Systems() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// configured is a Strategy built from one identifier_systems row.
type configured struct {
	cfg SystemConfig
	re  *regexp.Regexp
}

func newConfigured(row repository.IdentifierSystem) (*configured, error) {
	cfg, err := ParseSystemConfig(row)
	if err != nil {
		return nil, err
	}
	re, err := compilePattern(cfg.Pattern)
	if err != nil {
		return nil, err
	}
	return &configured{cfg: cfg, re: re}, nil
}

// System implements Strategy.
func (c *configured) System() string { return c.cfg.System }

// Detect implements Strategy: the anchored pattern matches the
// transformed value.
func (c *configured) Detect(raw string) bool {
	return c.re.MatchString(c.transform(raw))
}

// Normalize implements Strategy.
func (c *configured) Normalize(raw string) (string, error) {
	value := c.transform(raw)
	if !c.re.MatchString(value) {
		return "", &ValidationError{Msg: fmt.Sprintf("%s has an invalid format", c.cfg.DisplayName)}
	}
	if err := c.check(value); err != nil {
		return "", err
	}
	return value, nil
}

// transform canonicalizes raw according to the configured mode.
func (c *configured) transform(raw string) string {
	switch c.cfg.Transform {
	case TransformDigits:
		return digits(raw)
	case TransformUpper:
		return strings.ToUpper(collapseSpaces(strings.TrimSpace(raw)))
	case TransformLower:
		return strings.ToLower(collapseSpaces(strings.TrimSpace(raw)))
	default:
		return collapseSpaces(strings.TrimSpace(raw))
	}
}

// check validates the check digits of the normalized value.
func (c *configured) check(value string) error {
	if c.cfg.CheckAlgorithm == CheckNone || c.cfg.CheckBaseLen == 0 {
		return nil
	}
	if len(value) < c.cfg.CheckBaseLen+c.cfg.CheckDVCount {
		return &ValidationError{Msg: fmt.Sprintf("%s is too short for its check digits", c.cfg.DisplayName)}
	}
	if isDigit(value) && allEqual(value) {
		return &ValidationError{Msg: fmt.Sprintf("%s has an invalid sequence of digits", c.cfg.DisplayName)}
	}
	base := value[:c.cfg.CheckBaseLen]
	for dv := 1; dv <= c.cfg.CheckDVCount; dv++ {
		if !isDigit(base) {
			return &ValidationError{Msg: fmt.Sprintf("%s has an invalid check digit", c.cfg.DisplayName)}
		}
		expected := c.checkDigit(base, dv)
		if len(value) <= c.cfg.CheckBaseLen+dv-1 || value[c.cfg.CheckBaseLen+dv-1] != expected {
			return &ValidationError{Msg: fmt.Sprintf("%s has an invalid check digit", c.cfg.DisplayName)}
		}
		base += string(expected)
	}
	return nil
}

// checkDigit computes the dv-th check digit of base. Later check
// digits restart the descending weights one higher (CPF: 10..2 then
// 11..2), which is why the offset is passed down.
func (c *configured) checkDigit(base string, dv int) byte {
	switch c.cfg.CheckAlgorithm {
	case CheckMod11Cyclic:
		return checkMod11Cyclic(base)
	default: // CheckMod11Desc
		return checkMod11Descending(base, c.cfg.CheckStartWeight+dv-1)
	}
}

// compilePattern anchors the configured regex, so a pattern like
// "[0-9]{11}" matches the whole value and nothing else.
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
