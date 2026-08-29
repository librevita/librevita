package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/core/database/fle"
)

// PlanItem holds a structured plan activity belonging to an Episode.
type PlanItem struct {
	ent.Schema
}

// Fields of the PlanItem.
func (PlanItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Clinic tenant ID"),
		field.UUID("patient_id", uuid.UUID{}).
			Comment("Patient ID (denormalized for FLE / isolation)"),
		field.UUID("episode_id", uuid.UUID{}).
			Comment("Owning SOAP episode"),
		field.Enum("kind").
			Values("medication", "procedure", "exam", "appointment", "instruction").
			Default("instruction"),
		field.Enum("status").
			Values("draft", "active", "completed", "cancelled").
			Default("active"),
		field.Time("scheduled_at").
			Optional().
			Nillable(),

		field.String("code_system").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity code system (PHI)"),
		field.String("code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity code (PHI)"),
		field.String("display").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity display (PHI)"),
		field.String("description").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Plan narrative (PHI)"),

		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the PlanItem.
func (PlanItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("plan_items").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("plan_items").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("episode", Episode.Type).
			Ref("plan_items").
			Field("episode_id").
			Unique().
			Required(),
	}
}

// Indexes of the PlanItem.
func (PlanItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "episode_id"),
		index.Fields("patient_id", "created_at"),
		index.Fields("episode_id"),
	}
}
