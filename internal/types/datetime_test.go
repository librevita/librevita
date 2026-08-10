package types

import (
	"testing"
	"time"
)

// TestDateTime covers the canonical stored form, the accepted layouts
// (database DEFAULT and RFC3339Nano) and the round-trip through Time.
func TestDateTime(t *testing.T) {
	now := DateTimeFromTime(time.Date(2026, 8, 10, 17, 43, 37, 678000000, time.UTC))
	if got := now.String(); got != "2026-08-10T17:43:37.678Z" {
		t.Fatalf("DateTimeFromTime = %q, want canonical millis form", got)
	}
	if !now.Valid() {
		t.Fatal("canonical form must be valid")
	}
	parsed, err := now.Time()
	if err != nil || parsed.UnixNano() != time.Date(2026, 8, 10, 17, 43, 37, 678000000, time.UTC).UnixNano() {
		t.Fatalf("Time() = %v, %v", parsed, err)
	}

	for _, stored := range []string{
		"2026-08-10T17:43:37.678Z",       // canonical
		"2026-08-10T17:43:37.678123Z",    // strftime %f
		"2026-08-10T17:43:37.678123456Z", // RFC3339Nano (legacy sessions)
	} {
		d, ok := ParseDateTime(stored)
		if !ok || !d.Valid() {
			t.Fatalf("ParseDateTime(%q) must be valid", stored)
		}
	}

	if DateTime("2026-08-10").Valid() {
		t.Fatal("a plain date is not a timestamp")
	}
	if _, ok := ParseDateTime("17:43:37"); ok {
		t.Fatal("ParseDateTime must reject non-timestamp values")
	}

	// Now() must round-trip through the canonical layout.
	n := Now()
	if got, _ := n.Time(); got.IsZero() {
		t.Fatal("Now() must parse")
	}
	if _, ok := ParseDateTime(n.String()); !ok {
		t.Fatal("Now() must be in an accepted stored form")
	}

	// NULL (never-decided request) scans to the empty DateTime, and the
	// empty DateTime writes back as a real NULL, never as "".
	var d DateTime
	if err := d.Scan(nil); err != nil || d != "" {
		t.Fatalf("Scan(nil) = %q, %v; want empty", d, err)
	}
	v, err := d.Value()
	if err != nil || v != nil {
		t.Fatalf("Value() = %v, %v; want nil (real NULL)", v, err)
	}
	full := DateTimeFromTime(time.Date(2026, 8, 10, 17, 43, 37, 678000000, time.UTC))
	v, err = full.Value()
	if err != nil || v != "2026-08-10T17:43:37.678Z" {
		t.Fatalf("Value() = %v, %v; want canonical string", v, err)
	}
}
