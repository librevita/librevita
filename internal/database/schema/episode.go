package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/core/database/zk"
)

// Episode holds the schema definition for Medical Records, Clinical Notes, and Encounters.
// All clinical PHI (anamnesis, physical exam, diagnostic hypotheses, prescriptions, clinical notes)
// is stored encrypted transparently via ValueScanner.
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
			Comment("Attending physician / clinician ID"),
		field.UUID("appointment_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Linked appointment ID if applicable"),
		field.Enum("episode_type").
			Values("consultation", "anamnesis", "evolution", "prescription", "exam_request", "diagnostic").
			Default("consultation").
			Comment("Type of the episode"),
		field.Enum("status").
			Values("draft", "finalized", "archived").
			Default("draft").
			Comment("Lifecycle status of the episode"),

		// Confidential Clinical PHI Fields (Stored as BLOB/BYTEA in DB, strings in Go):
		field.String("notes").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Clinical notes / anamnesis / consultation details (stored as BLOB/BYTEA in DB)"),
		field.String("prescription").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Prescription and medical orders (stored as BLOB/BYTEA in DB)"),
		field.String("diagnostic").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Diagnostic hypotheses / CID (stored as BLOB/BYTEA in DB)"),

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
	}
}
