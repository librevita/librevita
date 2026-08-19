package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// Patient holds the schema definition for the Patient entity under LibreVita's Zero-Knowledge architecture.
type Patient struct {
	ent.Schema
}

// Mixin of the Patient entity.
func (Patient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.ZeroKnowledgeMixin{},
	}
}

// Fields of the Patient.
func (Patient) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Comment("Primary unique identifier (UUID v4/v7)"),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Tenant isolation: clinic ID reference"),
		field.String("status").
			Default("active").
			Comment("Lifecycle status: active, inactive, archived"),
		field.UUID("created_by", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("User ID who registered this patient"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("Record creation timestamp"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("Record last update timestamp"),
	}
}

// Edges of the Patient.
func (Patient) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("patients").
			Field("clinic_id").
			Unique().
			Required(),
		edge.To("identifiers", PatientIdentifier.Type),
		edge.To("appointments", Appointment.Type),
		edge.To("episodes", Episode.Type),
	}
}

// Indexes of the Patient.
func (Patient) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id"),
		index.Fields("clinic_id", "blind_index"),
		index.Fields("status"),
	}
}
