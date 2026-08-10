package types

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// DateTime is an ISO-8601 UTC timestamp stored in the SQLite TEXT
// columns (created_at, updated_at, expires_at, decided_at). The
// canonical form the application writes has millisecond precision
// (2026-08-10T17:43:37.678Z); values produced by the database DEFAULT
// (strftime '%Y-%m-%dT%H:%M:%fZ') and RFC3339Nano forms parse too.
//
// Being a string keeps the stored representation unchanged, while the
// type prevents mixing timestamps with plain dates (birth_date) or
// display strings at compile time.
type DateTime string

// utcMilliLayout is the canonical stored form.
const utcMilliLayout = "2006-01-02T15:04:05.000Z"

// datetimeLayouts lists the accepted stored forms, canonical first.
var datetimeLayouts = []string{utcMilliLayout, time.RFC3339Nano, time.RFC3339}

// Now returns the current instant in the canonical stored form.
func Now() DateTime {
	return DateTimeFromTime(time.Now())
}

// DateTimeFromTime converts an instant to the canonical stored form.
func DateTimeFromTime(t time.Time) DateTime {
	return DateTime(t.UTC().Format(utcMilliLayout))
}

// String returns the stored representation of d.
func (d DateTime) String() string {
	return string(d)
}

// Time parses d back into an instant, trying every accepted layout.
func (d DateTime) Time() (time.Time, error) {
	for _, layout := range datetimeLayouts {
		if t, err := time.Parse(layout, string(d)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Value: string(d), Layout: utcMilliLayout}
}

// Valid reports whether d is a parseable stored timestamp.
func (d DateTime) Valid() bool {
	_, err := d.Time()
	return err == nil
}

// ParseDateTime converts a stored value to the enum. ok is false when
// the value is not a parseable timestamp.
func ParseDateTime(s string) (DateTime, bool) {
	d := DateTime(s)
	return d, d.Valid()
}

// Scan implements sql.Scanner: a NULL column (e.g. a request that was
// never decided) becomes the empty DateTime instead of failing.
func (d *DateTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = ""
	case string:
		*d = DateTime(v)
	case []byte:
		*d = DateTime(v)
	default:
		return fmt.Errorf("types: cannot scan %T into DateTime", src)
	}
	return nil
}

// Value implements driver.Valuer so the canonical stored form is
// written as a plain string.
func (d DateTime) Value() (driver.Value, error) {
	return string(d), nil
}
