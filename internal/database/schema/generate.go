package schema

import (
	"fmt"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
)

//go:generate go run -mod=mod entc.go

var blobType = map[string]string{
	dialect.SQLite:   "blob",
	dialect.Postgres: "bytea",
}

// newUUIDv7 returns a new time-ordered UUIDv7 identifier.
func newUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("failed to generate UUIDv7: %v", err))
	}
	return id
}
