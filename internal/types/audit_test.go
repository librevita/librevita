package types

import "testing"

// TestResult mirrors the CHECK constraint on audit_log.result: only
// success and failure are accepted, and the round-trip through the
// stored representation preserves the value.
func TestResult(t *testing.T) {
	for _, r := range []Result{ResultSuccess, ResultFailure} {
		if !r.Valid() {
			t.Fatalf("%q must be valid", r)
		}
		got, ok := ParseResult(r.String())
		if !ok || got != r {
			t.Fatalf("ParseResult(%q) = %q, %v; want %q, true", r.String(), got, ok, r)
		}
	}
	if Result("successful").Valid() {
		t.Fatal("value outside the CHECK set must not be valid")
	}
	if _, ok := ParseResult("unknown"); ok {
		t.Fatal("ParseResult must reject values outside the CHECK set")
	}
}
