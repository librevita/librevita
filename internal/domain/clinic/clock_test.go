package clinic

import (
	"testing"
	"time"
)

func TestClockFormatsInClinicZone(t *testing.T) {
	clock := NewClock("America/Sao_Paulo")
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)

	if got, want := clock.FormatUI(utc), "2026-08-06 15:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
	if got, want := clock.Format(utc, time.RFC3339), "2026-08-06T15:04:05-03:00"; got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
}

func TestClockFormatsAnyLocationAsUTCInstant(t *testing.T) {
	clock := NewClock("Europe/London")
	local := time.Date(2026, 8, 6, 20, 4, 5, 0, time.FixedZone("X", 2*60*60))

	if got, want := clock.FormatUI(local), "2026-08-06 19:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
}

func TestClockFallsBackToUTCOnUnknownZone(t *testing.T) {
	clock := NewClock("Mars/Olympus")
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)

	if got, want := clock.Format(utc, time.RFC3339), "2026-08-06T18:04:05Z"; got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
}
