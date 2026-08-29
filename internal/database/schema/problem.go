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

// Problem holds a structured assessment diagnosis belonging to an Episode.
type Problem struct {
	ent.Schema
}

// Fields of the Problem.
func (Problem) Fields() []ent.Field {
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

		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
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

// Indexes of the Problem.
func (Problem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "episode_id"),
		index.Fields("patient_id", "created_at"),
		index.Fields("episode_id"),
	}
}
