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

// Patient holds the schema definition for the Patient entity.
type Patient struct {
	ent.Schema
}

// Fields of the Patient.
func (Patient) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable().
			Comment("Primary unique identifier (UUIDv7)"),
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Tenant isolation: clinic ID reference"),
		field.Enum("status").
			Values("active", "inactive", "archived").
			Default("active").
			Comment("Lifecycle status of the patient"),

		// Patient PII/PHI Fields (Stored as BLOB/BYTEA in DB, pure strings in Go):
		field.String("display_name").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			NotEmpty().
			Annotations(zk.Searchable()).
			Comment("Patient full / social name (stored as BLOB/BYTEA in DB)"),

		field.String("phone").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			NotEmpty().
			Annotations(zk.Searchable()).
			Comment("Patient contact phone (stored as BLOB/BYTEA in DB)"),

		field.String("email").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			NotEmpty().
			Annotations(zk.Searchable()).
			Comment("Patient email address (stored as BLOB/BYTEA in DB)"),

		field.String("birth_date").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Birth date YYYY-MM-DD (stored as BLOB/BYTEA in DB)"),

		field.String("sex").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Patient sex / identity (stored as BLOB/BYTEA in DB)"),

		field.String("street").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Street address (stored as BLOB/BYTEA in DB)"),

		field.String("city").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("City (stored as BLOB/BYTEA in DB)"),

		field.String("state").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("State / Province (stored as BLOB/BYTEA in DB)"),

		field.String("postal_code").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Postal code / CEP (stored as BLOB/BYTEA in DB)"),

		field.String("notes").
			SchemaType(blobType).
			ValueScanner(zk.EncryptedString()).
			Optional().
			Comment("Clinical / general notes (stored as BLOB/BYTEA in DB)"),

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
		index.Fields("status"),
	}
}
