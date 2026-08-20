package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/policy"
)

// TestPolicyOrigin mirrors the CHECK constraint on
// policy_versions.origin: only seed/admin/system are valid, and the
// stored representation round-trips.
func TestPolicyOrigin(t *testing.T) {
	for _, origin := range []policy.PolicyOrigin{policy.PolicyOriginSeed, policy.PolicyOriginAdmin, policy.PolicyOriginSystem} {
		assert.True(t, origin.Valid(), "%q must be valid", origin)
		got, ok := policy.ParsePolicyOrigin(origin.String())
		assert.True(t, ok)
		assert.Equal(t, origin, got)
	}
	assert.False(t, policy.PolicyOrigin("cli").Valid())
	_, ok := policy.ParsePolicyOrigin("cli")
	assert.False(t, ok)
}
