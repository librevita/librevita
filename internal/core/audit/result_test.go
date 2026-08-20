package audit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResult mirrors the CHECK constraint on audit_log.result: only
// success and failure are accepted, and the round-trip through the
// stored representation preserves the value.
func TestResult(t *testing.T) {
	for _, r := range []AuditResult{AuditResultSuccess, AuditResultFailure} {
		assert.True(t, r.Valid(), "%q must be valid", r)
		got, ok := ParseAuditResult(r.String())
		assert.True(t, ok)
		assert.Equal(t, r, got)
	}
	assert.False(t, AuditResult("successful").Valid())
	_, ok := ParseAuditResult("unknown")
	assert.False(t, ok)
}
