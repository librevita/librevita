package mixin

import (
	"database/sql/driver"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"

	"librevita.org/pkg/ident"
)

// UUID defines the UUIDv7 primary key for an entity as defined type T.
type UUID[T interface {
	~[16]byte
	driver.Valuer
}] struct {
	mixin.Schema
}

// Fields of the UUID mixin.
func (UUID[T]) Fields() []ent.Field {
	var zero T
	return []ent.Field{
		field.UUID("id", zero).
			Default(ident.New[T]).
			Immutable(),
	}
}
