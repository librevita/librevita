// Package host classifies HTTP Host values against the configured base domain.
package host

import (
	"net"
	"strings"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/clinicctx"
)

var (
	// ErrInvalidHost is returned when Host is not apex, www, or a single slug label.
	ErrInvalidHost = errors.New("host: invalid host for base domain")
)

// Kind is the kind of Host after classification.
type Kind int

const (
	// KindApex is base_domain or www.base_domain.
	KindApex Kind = iota
	// KindClinic is {slug}.base_domain.
	KindClinic
)

// Result is a classified Host.
type Result struct {
	Kind Kind
	Slug string
}

// Classify parses Host against baseDomain. Port is stripped. Only the apex,
// www, or a single DNS label plus the base domain are accepted.
func Classify(rawHost, baseDomain string) (Result, error) {
	host := strings.ToLower(strings.TrimSpace(rawHost))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	base := strings.ToLower(strings.TrimSpace(baseDomain))
	if host == "" || base == "" {
		return Result{}, ErrInvalidHost
	}
	if host == base || host == "www."+base {
		return Result{Kind: KindApex}, nil
	}
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return Result{}, ErrInvalidHost
	}
	slug := strings.TrimSuffix(host, suffix)
	if slug == "" || strings.Contains(slug, ".") || clinicctx.IsReservedSlug(slug) {
		return Result{}, ErrInvalidHost
	}
	return Result{Kind: KindClinic, Slug: slug}, nil
}
