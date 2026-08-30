package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// Clinic defines the owning clinic tenant ID field.
type Clinic struct {
	mixin.Schema
}

// Fields of the Clinic mixin.
func (Clinic) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Clinic tenant ID"),
	}
}
