package acme

import (
	"context"
	"net"
	"strings"
	"time"

	"librevita.org/pkg/errors"
	"librevita.org/pkg/log"
)

// DNSProvider provisions and cleans up DNS TXT records for DNS-01 challenges.
type DNSProvider interface {
	// Present creates or updates the _acme-challenge.<domain> TXT record with keyAuthRecord.
	Present(ctx context.Context, domain, keyAuthRecord string) error

	// CleanUp removes the _acme-challenge.<domain> TXT record.
	CleanUp(ctx context.Context, domain, keyAuthRecord string) error
}

// ChallengeRecordName returns the standard ACME DNS challenge record name: _acme-challenge.<domain>.
func ChallengeRecordName(domain string) string {
	d := strings.TrimPrefix(domain, "*.")
	d = strings.TrimPrefix(d, ".")
	return "_acme-challenge." + d
}

// TXTLookupFunc is the signature for looking up TXT records, pluggable for testing.
type TXTLookupFunc func(ctx context.Context, name string) ([]string, error)

// DefaultResolverLookup is the standard net.DefaultResolver TXT lookup.
func DefaultResolverLookup(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}

// WaitForDNSPropagation polls DNS until the expected TXT record is visible or timeout expires.
func WaitForDNSPropagation(ctx context.Context, domain, expectedRecord string, timeout time.Duration, lookup TXTLookupFunc, logger log.Logger) error {
	recordName := ChallengeRecordName(domain)
	if lookup == nil {
		lookup = DefaultResolverLookup
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		records, err := lookup(ctx, recordName)
		if err == nil {
			for _, r := range records {
				if r == expectedRecord {
					logger.DebugContext(ctx, "acme: dns-01 record verified in dns",
						log.String("record", recordName),
						log.String("expected", expectedRecord),
					)
					return nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "acme: dns propagation context canceled")
		case <-timeoutChan:
			logger.WarnContext(ctx, "acme: dns propagation wait timed out; proceeding with challenge",
				log.String("record", recordName),
				log.String("expected", expectedRecord),
			)
			return nil // Proceed anyway to let Let's Encrypt attempt validation
		case <-ticker.C:
		}
	}
}
