package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

// Finding holds a structured objective finding belonging to an Episode.
type Finding struct {
	ent.Schema
}

// Mixin of the Finding.
func (Finding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID[ident.FindingID]{},
		mixin.Clinic{},
		mixin.ClinicalChild{},
		mixin.Time{},
	}
}

// Fields of the Finding.
func (Finding) Fields() []ent.Field {
	return []ent.Field{
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
