// Package ident holds defined UUID types for LibreVita entities.
//
// Each entity primary key is a distinct type so the compiler rejects a
// PatientID where a ClinicID is required. Columns in the database stay UUID.
//
// types.go is the source of truth: every declaration must be
// `type NameID uuid.UUID` (defined type, not alias). go generate writes
// codec_gen.go (gitignored; task gen).
package ident

import (
	"fmt"

	"github.com/google/uuid"
)

// uuidID is the underlying shape of google/uuid.UUID and of every defined
// identifier in this package.
type uuidID interface {
	~[16]byte
}

// New returns a UUIDv7 identifier of type T.
func New[T uuidID]() T {
	u, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("id: generate UUIDv7: %v", err))
	}
	return T(u)
}

// FromUUID converts a raw UUID to T.
func FromUUID[T uuidID](u uuid.UUID) T {
	return T(u)
}

// Parse converts a UUID string to T.
func Parse[T uuidID](s string) (T, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		var zero T
		return zero, err
	}
	return T(u), nil
}

// MustParse converts a UUID string to T and panics on error.
func MustParse[T uuidID](s string) T {
	id, err := Parse[T](s)
	if err != nil {
		panic(err)
	}
	return id
}
