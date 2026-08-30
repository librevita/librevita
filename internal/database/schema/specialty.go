package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"librevita.org/internal/database/schema/mixin"
)

// Specialty holds the schema definition for the Specialty entity.
type Specialty struct {
	ent.Schema
}

// Mixin of the Specialty.
func (Specialty) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.CreatedAt{},
	}
}

// Fields of the Specialty.
func (Specialty) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("Specialty name: Cardiologia, Pediatria, etc."),
	}
}

// Edges of the Specialty.
func (Specialty) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("specialties").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("users", User.Type).
			Ref("specialties"),
	}
}

// Indexes of the Specialty.
func (Specialty) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "name").
			Unique(),
	}
}
