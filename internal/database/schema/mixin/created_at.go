package mixin

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// CreatedAt defines an immutable creation timestamp for append-only or versioned entities.
type CreatedAt struct {
	mixin.Schema
}

// Fields of the CreatedAt mixin.
func (CreatedAt) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}
