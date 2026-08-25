package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/core/database/fle"
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
		field.UUID("user_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Optional portal login (users.clinic_id must match); unique per clinic when set"),
		field.Enum("status").
			Values("active", "inactive", "archived").
			Default("active").
			Comment("Lifecycle status of the patient"),

		// Patient PII/PHI Fields (Stored as BLOB/BYTEA in DB, pure strings in Go):
		field.String("display_name").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			NotEmpty().
			Annotations(fle.SearchableName()).
			Comment("Patient full / social name (stored as BLOB/BYTEA in DB)"),

		field.String("phone").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			NotEmpty().
			Annotations(fle.SearchablePhone()).
			Comment("Patient contact phone (stored as BLOB/BYTEA in DB)"),

		field.String("email").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			NotEmpty().
			Annotations(fle.SearchableEmail()).
			Comment("Patient email address (stored as BLOB/BYTEA in DB)"),

		field.String("birth_date").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Birth date YYYY-MM-DD (stored as BLOB/BYTEA in DB)"),

		field.String("sex").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Patient sex / identity (stored as BLOB/BYTEA in DB)"),

		field.String("street").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Street address (stored as BLOB/BYTEA in DB)"),

		field.String("city").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("City (stored as BLOB/BYTEA in DB)"),

		field.String("state").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("State / Province (stored as BLOB/BYTEA in DB)"),

		field.String("postal_code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Postal code / CEP (stored as BLOB/BYTEA in DB)"),

		field.String("notes").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
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
		edge.From("portal_user", User.Type).
			Ref("portal_patient").
			Field("user_id").
			Unique(),
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
		index.Fields("clinic_id", "user_id").
			Unique().
			Annotations(entsql.IndexWhere("user_id IS NOT NULL")),
	}
}
