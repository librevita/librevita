package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Registry is the runtime view of identifier_systems: a thread-safe
// cache of the active system configurations. SystemsService reloads it
// after every change, so a new document type is usable immediately
// without restarting the application.
type Registry struct {
	mu      sync.RWMutex
	systems map[string]*Configured
	order   []string
}

// NewRegistry builds an empty registry. It contains no systems until
// Reload loads them from the database.
func NewRegistry() *Registry {
	return &Registry{systems: map[string]*Configured{}}
}

// Reload replaces the cached system list from the database. It is
// called at boot and after every administrative change. Detection
// order is deterministic regardless of the query's own ordering:
// longest pattern first, then by URN.
func (r *Registry) Reload(systems []*IdentifierSystem) error {
	sorted := append([]*IdentifierSystem(nil), systems...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if len(sorted[i].Pattern) != len(sorted[j].Pattern) {
			return len(sorted[i].Pattern) > len(sorted[j].Pattern)
		}
		return sorted[i].System < sorted[j].System
	})

	next := make(map[string]*Configured, len(sorted))
	order := make([]string, 0, len(sorted))
	for _, row := range sorted {
		c, err := NewConfigured(row)
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

// Configured is a Strategy built from one identifier_systems row.
type Configured struct {
	Cfg SystemConfig
	Re  *regexp.Regexp
}

// NewConfigured builds a Configured strategy from an IdentifierSystem row.
func NewConfigured(row *IdentifierSystem) (*Configured, error) {
	cfg, err := ParseSystemConfig(row)
	if err != nil {
		return nil, err
	}
	re, err := compilePattern(cfg.Pattern)
	if err != nil {
		return nil, err
	}
	return &Configured{Cfg: cfg, Re: re}, nil
}

// System implements Strategy.
func (c *Configured) System() string { return c.Cfg.System }

// Detect implements Strategy: the anchored pattern matches the
// transformed value.
func (c *Configured) Detect(raw string) bool {
	return c.Re.MatchString(c.transform(raw))
}

// Normalize implements Strategy.
func (c *Configured) Normalize(raw string) (string, error) {
	value := c.transform(raw)
	if !c.Re.MatchString(value) {
		return "", &ValidationError{Msg: fmt.Sprintf("%s has an invalid format", c.Cfg.DisplayName)}
	}
	if err := c.check(value); err != nil {
		return "", err
	}
	return value, nil
}

// transform canonicalizes raw according to the configured mode.
func (c *Configured) transform(raw string) string {
	switch c.Cfg.Transform {
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
func (c *Configured) check(value string) error {
	if c.Cfg.CheckAlgorithm == CheckNone || c.Cfg.CheckBaseLen == 0 {
		return nil
	}
	if len(value) < c.Cfg.CheckBaseLen+c.Cfg.CheckDVCount {
		return &ValidationError{Msg: fmt.Sprintf("%s is too short for its check digits", c.Cfg.DisplayName)}
	}
	if isDigit(value) && allEqual(value) {
		return &ValidationError{Msg: fmt.Sprintf("%s has an invalid sequence of digits", c.Cfg.DisplayName)}
	}
	base := value[:c.Cfg.CheckBaseLen]
	for dv := 1; dv <= c.Cfg.CheckDVCount; dv++ {
		if !isDigit(base) {
			return &ValidationError{Msg: fmt.Sprintf("%s has an invalid check digit", c.Cfg.DisplayName)}
		}
		expected := c.checkDigit(base, dv)
		if len(value) <= c.Cfg.CheckBaseLen+dv-1 || value[c.Cfg.CheckBaseLen+dv-1] != expected {
			return &ValidationError{Msg: fmt.Sprintf("%s has an invalid check digit", c.Cfg.DisplayName)}
		}
		base += string(expected)
	}
	return nil
}

// checkDigit computes the dv-th check digit of base. Later check
// digits restart the descending weights one higher (CPF: 10..2 then
// 11..2), which is why the offset is passed down.
func (c *Configured) checkDigit(base string, dv int) byte {
	switch c.Cfg.CheckAlgorithm {
	case CheckMod11Cyclic:
		return checkMod11Cyclic(base)
	default: // CheckMod11Desc
		return checkMod11Descending(base, c.Cfg.CheckStartWeight+dv-1)
	}
}
