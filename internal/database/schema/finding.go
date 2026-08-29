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

// Finding holds a structured objective finding belonging to an Episode.
type Finding struct {
	ent.Schema
}

// Fields of the Finding.
func (Finding) Fields() []ent.Field {
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
		field.Enum("status").
			Values("recorded", "provisional", "cancelled").
			Default("recorded").
			Comment("Clinical state of the finding"),
		field.Enum("value_kind").
			Values("quantity", "string", "boolean", "coded").
			Comment("How the finding value is represented"),
		field.Time("effective_at").
			Default(time.Now).
			Comment("When the finding was observed"),

		field.String("code_system").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Finding code system (PHI)"),
		field.String("code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Annotations(fle.SearchableDocument()).
			Comment("Finding code (PHI)"),
		field.String("display").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Finding display (PHI)"),
		field.String("value_number").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Quantity value as decimal text (PHI)"),
		field.String("value_unit").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Quantity unit display (PHI)"),
		field.String("value_ucum").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Quantity UCUM code (PHI)"),
		field.String("value_text").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("String value (PHI)"),
		field.String("value_bool").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Boolean value as true/false (PHI)"),
		field.String("value_coded_system").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Coded value system (PHI)"),
		field.String("value_coded_code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Coded value code (PHI)"),
		field.String("value_coded_display").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Coded value display (PHI)"),

		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Finding.
func (Finding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("findings").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("findings").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("episode", Episode.Type).
			Ref("findings").
			Field("episode_id").
			Unique().
			Required(),
	}
}

// Indexes of the Finding.
func (Finding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "episode_id"),
		index.Fields("patient_id", "created_at"),
		index.Fields("episode_id"),
	}
}
