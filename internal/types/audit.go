// Package types holds domain-wide value types shared by the core and
// domain packages. The enums here mirror the CHECK constraints of the
// SQLite schema, so the set of values the application can produce is
// closed in the same way the database closes it.
package types

// AuditResult is the outcome of an audited operation. It mirrors the
// CHECK constraint on audit_log.result (see db/migrations/00003_audit.sql),
// so a value that is not one of the options below is a programming
// error, not a legitimate audit event.
type AuditResult string

const (
	// AuditResultSuccess marks an operation that completed.
	AuditResultSuccess AuditResult = "success"
	// AuditResultFailure marks an operation that was denied or errored.
	AuditResultFailure AuditResult = "failure"
)

// Valid reports whether r is one of the options the database CHECK
// constraint accepts.
func (r AuditResult) Valid() bool {
	return r == AuditResultSuccess || r == AuditResultFailure
}

// String returns the stored representation of r.
func (r AuditResult) String() string {
	return string(r)
}

// ParseAuditResult converts a stored value back to the enum. ok is false
// when the value is not one of the CHECK options.
func ParseAuditResult(s string) (AuditResult, bool) {
	r := AuditResult(s)
	return r, r.Valid()
}
