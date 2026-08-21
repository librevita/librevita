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

// Appointment holds the schema definition for the Appointment entity.
// Sensitive appointment details (reason for visit, clinical triage notes)
// are stored encrypted transparently via ValueScanner.
type Appointment struct {
	ent.Schema
}

// Fields of the Appointment.
func (Appointment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Clinic tenant ID"),
		field.UUID("patient_id", uuid.UUID{}).
			Comment("Patient ID"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Attending physician / clinician user ID"),
		field.Time("start_time").
			Comment("Scheduled start timestamp"),
		field.Time("end_time").
			Comment("Scheduled end timestamp"),
		field.Enum("status").
			Values("scheduled", "confirmed", "in_progress", "completed", "cancelled", "no_show").
			Default("scheduled").
			Comment("Status of the appointment"),

		// Confidential Appointment Fields (Stored as BLOB/BYTEA in DB, strings in Go):
		field.String("reason").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Reason for visit / triage description (stored as BLOB/BYTEA in DB)"),
		field.String("notes").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Clinical appointment notes (stored as BLOB/BYTEA in DB)"),

		field.UUID("created_by", uuid.UUID{}).
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Appointment.
func (Appointment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("appointments").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("appointments").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("appointments").
			Field("user_id").
			Unique().
			Required(),
		edge.To("episodes", Episode.Type),
	}
}

// Indexes of the Appointment.
func (Appointment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "start_time"),
		index.Fields("user_id", "start_time"),
		index.Fields("patient_id", "start_time"),
		index.Fields("status"),
	}
}
