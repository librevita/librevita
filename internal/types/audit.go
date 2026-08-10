// Package types holds domain-wide value types shared by the core and
// domain packages. The enums here mirror the CHECK constraints of the
// SQLite schema, so the set of values the application can produce is
// closed in the same way the database closes it.
package types

// Result is the outcome of an audited operation. It mirrors the CHECK
// constraint on audit_log.result (see db/migrations/00003_audit.sql),
// so a value that is not one of the options below is a programming
// error, not a legitimate audit event.
type Result string

const (
	// ResultSuccess marks an operation that completed.
	ResultSuccess Result = "success"
	// ResultFailure marks an operation that was denied or errored.
	ResultFailure Result = "failure"
)

// Valid reports whether r is one of the options the database CHECK
// constraint accepts.
func (r Result) Valid() bool {
	return r == ResultSuccess || r == ResultFailure
}

// String returns the stored representation of r.
func (r Result) String() string {
	return string(r)
}

// ParseResult converts a stored value back to the enum. ok is false when
// the value is not one of the CHECK options.
func ParseResult(s string) (Result, bool) {
	r := Result(s)
	return r, r.Valid()
}
