package schema

import (
	"fmt"

	"github.com/google/uuid"
)

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --target=../../../ent --feature sql/versioned-migration .

// newUUIDv7 returns a new time-ordered UUIDv7 identifier.
func newUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("failed to generate UUIDv7: %v", err))
	}
	return id
}
