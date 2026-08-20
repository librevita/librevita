package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClockFormatsInClinicZone(t *testing.T) {
	clock := NewClock("America/Sao_Paulo")
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)

	assert.Equal(t, "2026-08-06 15:04", clock.FormatUI(utc))
	assert.Equal(t, "2026-08-06T15:04:05-03:00", clock.Format(utc, time.RFC3339))
}

func TestClockFormatsAnyLocationAsUTCInstant(t *testing.T) {
	clock := NewClock("Europe/London")
	local := time.Date(2026, 8, 6, 20, 4, 5, 0, time.FixedZone("X", 2*60*60))

	assert.Equal(t, "2026-08-06 19:04", clock.FormatUI(local))
}

func TestClockFallsBackToUTCOnUnknownZone(t *testing.T) {
	clock := NewClock("Mars/Olympus")
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)

	assert.Equal(t, "2026-08-06T18:04:05Z", clock.Format(utc, time.RFC3339))
}
