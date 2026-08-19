package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Specialty holds the schema definition for the Specialty entity.
type Specialty struct {
	ent.Schema
}

// Fields of the Specialty.
func (Specialty) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Clinic tenant ID"),
		field.String("name").
			NotEmpty().
			Comment("Specialty name: Cardiologia, Pediatria, etc."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
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
