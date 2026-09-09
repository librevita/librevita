// Package host classifies HTTP Host values against the configured base domain.
package host

import (
	"net"
	"regexp"
	"strings"

	"librevita.org/pkg/errors"
)

var (
	// ErrInvalidHost is returned when Host is invalid or empty.
	ErrInvalidHost = errors.New("host: invalid host format or empty")

	hostRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

func isValidHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	return hostRE.MatchString(h)
}

// Kind is the kind of Host after classification.
type Kind int

const (
	// KindApex is base_domain or www.base_domain.
	KindApex Kind = iota
	// KindClinic is a clinic-specific custom domain.
	KindClinic
)

// Result is a classified Host.
type Result struct {
	Kind   Kind
	Domain string
	Slug   string // deprecated: alias for Domain
}

// Classify parses Host against baseDomain. Port is stripped.
// Apex and www are classified as KindApex. Any other valid domain is
// classified as KindClinic with Domain set.
func Classify(rawHost, baseDomain string) (Result, error) {
	host := strings.ToLower(strings.TrimSpace(rawHost))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	base := strings.ToLower(strings.TrimSpace(baseDomain))
	if host == "" || base == "" || !isValidHostname(host) || !isValidHostname(base) {
		return Result{}, ErrInvalidHost
	}
	if host == base || host == "www."+base {
		return Result{Kind: KindApex}, nil
	}
	return Result{Kind: KindClinic, Domain: host, Slug: host}, nil
}
