package clinic

import "time"

// TimeLayout is the human-readable layout used across the UI.
// Instants are rendered in the clinic's timezone.
const TimeLayout = "2006-01-02 15:04"

// Clock renders instants in the clinic's timezone. Stored instants are
// always UTC; Format converts them for display.
type Clock struct {
	loc *time.Location
}

// NewClock builds a Clock for an IANA zone, falling back to UTC when the
// zone is unknown.
func NewClock(zone string) *Clock {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	return &Clock{loc: loc}
}

// Format renders an instant (any location) in the clock's zone.
func (c *Clock) Format(t time.Time, layout string) string {
	return t.UTC().In(c.loc).Format(layout)
}

// FormatUI renders an instant with the default UI layout in the clinic's
// timezone.
func (c *Clock) FormatUI(t time.Time) string {
	return c.Format(t, TimeLayout)
}

// utcMilliLayout matches the timestamps written by the database
// (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')).
const utcMilliLayout = "2006-01-02T15:04:05.000Z"

// FormatStored renders a database timestamp in the clinic's timezone,
// keeping the raw value when it cannot be parsed.
func (c *Clock) FormatStored(stored string) string {
	t, err := time.Parse(utcMilliLayout, stored)
	if err != nil {
		return stored
	}
	return c.FormatUI(t)
}
