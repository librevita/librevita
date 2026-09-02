package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

// Patient holds the schema definition for the Patient entity.
type Patient struct {
	ent.Schema
}

// Mixin of the Patient.
func (Patient) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID[ident.PatientID]{},
		mixin.Clinic{},
		mixin.Time{},
	}
}

// Fields of the Patient.
func (Patient) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", ident.UserID{}).
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

		field.UUID("created_by", ident.UserID{}).
			Optional().
			Nillable().
			Comment("User ID who registered this patient"),
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
		edge.To("findings", Finding.Type),
		edge.To("problems", Problem.Type),
		edge.To("plan_items", PlanItem.Type),
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
