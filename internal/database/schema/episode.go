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

// Episode holds the schema definition for a SOAP clinical note (one encounter).
// Narrative SOAP sections and structured children are encrypted with the Patient DEK.
type Episode struct {
	ent.Schema
}

// Fields of the Episode.
func (Episode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Clinic tenant ID"),
		field.UUID("patient_id", uuid.UUID{}).
			Comment("Patient ID"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Attending physician / author ID"),
		field.UUID("appointment_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Linked appointment ID if applicable"),
		field.UUID("predecessor_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Finalized episode this note amends"),
		field.Enum("episode_type").
			Values("consultation", "anamnesis", "evolution", "prescription", "exam_request", "diagnostic").
			Default("consultation").
			Comment("Type of the episode"),
		field.Enum("status").
			Values("draft", "finalized", "archived").
			Default("draft").
			Comment("Lifecycle status of the episode"),
		field.Enum("class").
			Values("ambulatory", "emergency", "inpatient", "virtual").
			Default("ambulatory").
			Comment("Care setting of the visit"),
		field.Time("occurred_at").
			Default(time.Now).
			Comment("Encounter start / note time"),
		field.Time("ended_at").
			Optional().
			Nillable().
			Comment("Encounter end time if known"),

		field.String("subjective").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("SOAP Subjective narrative (BLOB/BYTEA)"),
		field.String("objective").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("SOAP Objective narrative (BLOB/BYTEA)"),
		field.String("assessment").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("SOAP Assessment narrative (BLOB/BYTEA)"),
		field.String("plan").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("SOAP Plan narrative (BLOB/BYTEA)"),

		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Episode.
func (Episode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("episodes").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("episodes").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("episodes").
			Field("user_id").
			Unique().
			Required(),
		edge.From("appointment", Appointment.Type).
			Ref("episodes").
			Field("appointment_id").
			Unique(),
		edge.To("amendment", Episode.Type).
			Unique(),
		edge.From("predecessor", Episode.Type).
			Ref("amendment").
			Unique().
			Field("predecessor_id"),
		edge.To("findings", Finding.Type),
		edge.To("problems", Problem.Type),
		edge.To("plan_items", PlanItem.Type),
	}
}

// Indexes of the Episode.
func (Episode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "created_at"),
		index.Fields("patient_id", "created_at"),
		index.Fields("user_id", "created_at"),
		index.Fields("episode_type"),
		index.Fields("status"),
		index.Fields("predecessor_id").
			Unique(),
	}
}
