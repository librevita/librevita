package mixin

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// UUID defines the standard UUIDv7 primary key for entities.
type UUID struct {
	mixin.Schema
}

// Fields of the UUID mixin.
func (UUID) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
	}
}

func newUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("failed to generate UUIDv7: %v", err))
	}
	return id
}
