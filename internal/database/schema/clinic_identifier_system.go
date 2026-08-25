package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ClinicIdentifierSystem is a clinic's opt-in to a global identifier system.
type ClinicIdentifierSystem struct {
	ent.Schema
}

// Annotations of the ClinicIdentifierSystem.
func (ClinicIdentifierSystem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "clinic_identifier_systems"},
	}
}

// Fields of the ClinicIdentifierSystem.
func (ClinicIdentifierSystem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.UUID("clinic_id", uuid.UUID{}),
		field.UUID("identifier_system_id", uuid.UUID{}),
	}
}

// Edges of the ClinicIdentifierSystem.
func (ClinicIdentifierSystem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("identifier_systems").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("system", IdentifierSystem.Type).
			Ref("clinic_opt_ins").
			Field("identifier_system_id").
			Unique().
			Required(),
	}
}

// Indexes of the ClinicIdentifierSystem.
func (ClinicIdentifierSystem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "identifier_system_id").
			Unique(),
	}
}
