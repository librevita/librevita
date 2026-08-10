package types

import "testing"

// TestPolicyOrigin mirrors the CHECK constraint on
// policy_versions.origin: only seed/admin/system are valid, and the
// stored representation round-trips.
func TestPolicyOrigin(t *testing.T) {
	for _, origin := range []PolicyOrigin{PolicyOriginSeed, PolicyOriginAdmin, PolicyOriginSystem} {
		if !origin.Valid() {
			t.Fatalf("%q must be valid", origin)
		}
		if got, ok := ParsePolicyOrigin(origin.String()); !ok || got != origin {
			t.Fatalf("ParsePolicyOrigin(%q) = %q, %v; want %q, true", origin.String(), got, ok, origin)
		}
	}
	if PolicyOrigin("cli").Valid() {
		t.Fatal("origin outside the CHECK set must not be valid")
	}
	if _, ok := ParsePolicyOrigin("cli"); ok {
		t.Fatal("ParsePolicyOrigin must reject values outside the CHECK set")
	}
}
