package ident

import (
	"database/sql/driver"

	"github.com/google/uuid"
)

func asUUID[T uuidID](id T) uuid.UUID { return uuid.UUID(id) }

func scanID[T uuidID](id *T, src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}
	*id = T(u)
	return nil
}

func valueID[T uuidID](id T) (driver.Value, error) {
	return asUUID(id).Value()
}

func marshalTextID[T uuidID](id T) ([]byte, error) {
	return asUUID(id).MarshalText()
}

func unmarshalTextID[T uuidID](id *T, data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}
	*id = T(u)
	return nil
}
