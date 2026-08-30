package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/database/schema/mixin"
)

// Problem holds a structured assessment diagnosis belonging to an Episode.
type Problem struct {
	ent.Schema
}

// Mixin of the Problem.
func (Problem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.ClinicalChild{},
		mixin.Time{},
	}
}

// Fields of the Problem.
func (Problem) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("clinical_status").
			Values("active", "inactive", "resolved").
			Default("active"),
		field.Enum("verification_status").
			Values("confirmed", "suspected", "refuted", "error").
			Default("confirmed"),
		field.Enum("category").
			Values("encounter", "list").
			Default("encounter"),
		field.Int("rank").
			Default(1).
			Positive().
			Comment("1 is the principal diagnosis"),

		field.String("code_system").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Diagnosis code system (PHI)"),
		field.String("code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Annotations(fle.SearchableDocument()).
			Comment("Diagnosis code (PHI)"),
		field.String("display").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Diagnosis display (PHI)"),
		field.String("text").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Diagnosis narrative (PHI)"),
	}
}

// Edges of the Problem.
func (Problem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("problems").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("problems").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("episode", Episode.Type).
			Ref("problems").
			Field("episode_id").
			Unique().
			Required(),
	}
}
