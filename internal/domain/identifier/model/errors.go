package model

import "errors"

// Domain errors for identifier systems.
var (
	ErrSystemNotFound      = errors.New("identifier: identifier system not found")
	ErrDuplicate           = errors.New("identifier: identifier system already exists")
	ErrDuplicateIdentifier = errors.New("identifier: identifier already registered")
	ErrSystemInactive      = errors.New("identifier: identifier system is inactive")
	ErrSystemImmutable     = errors.New("identifier: cannot modify system identifier")
	ErrNotFound            = errors.New("identifier: identifier not found")
	ErrValueRequired       = errors.New("identifier: value is required")
	ErrSystemNotAllowed    = errors.New("identifier: system is not enabled for this clinic")
)

// ValidationError is returned for values that do not fit the system's
// scheme (bad check digit, wrong length, ...).
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string {
	return e.Msg
}
