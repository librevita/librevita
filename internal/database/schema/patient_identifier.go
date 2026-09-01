package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// PatientIdentifier holds the schema definition for encrypted Patient Identifiers.
type PatientIdentifier struct {
	ent.Schema
}

// Mixin of the PatientIdentifier.
func (PatientIdentifier) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.Time{},
	}
}

// Fields of the PatientIdentifier.
func (PatientIdentifier) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("patient_id", uuid.UUID{}).
			Comment("Belonging patient ID"),
		field.String("system").
			NotEmpty().
			Comment("System URN reference"),
		field.Bytes("value_ciphertext").
			NotEmpty().
			Comment("Self-describing XChaCha20-Poly1305 envelope of the document value"),
		field.String("blind_index").
			MaxLen(88).
			NotEmpty().
			Comment("Keyed blind index: algorithm$<scope><purpose>$kid$hex"),
		field.UUID("created_by", uuid.UUID{}).
			Optional().
			Nillable(),
	}
}

// Edges of the PatientIdentifier.
func (PatientIdentifier) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("patient_identifiers").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("identifiers").
			Field("patient_id").
			Unique().
			Required(),
		edge.To("identifier_system", IdentifierSystem.Type).
			Unique(),
	}
}

// Indexes of the PatientIdentifier.
func (PatientIdentifier) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("patient_id"),
		index.Fields("system"),
		index.Fields("clinic_id", "blind_index").
			Unique(),
	}
}
