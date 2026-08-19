package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// IdentifierSystem holds the schema definition for the IdentifierSystem catalog.
type IdentifierSystem struct {
	ent.Schema
}

// Fields of the IdentifierSystem.
func (IdentifierSystem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("system").
			NotEmpty().
			Unique().
			Comment("URN system identifier: urn:librevita:id:br:cpf, etc."),
		field.String("display_name").
			NotEmpty().
			Comment("Human readable system name: CPF (Brasil)"),
		field.String("pattern").
			NotEmpty().
			Comment("Regex validation pattern"),
		field.String("transform").
			Default("none").
			Comment("Canonicalization transform: none, digits, upper, lower"),
		field.String("check_algorithm").
			Default("none").
			Comment("Checksum algorithm: none, mod11_desc, mod11_cyclic"),
		field.Int("check_base_len").
			Default(0),
		field.Int("check_dv_count").
			Default(1),
		field.Int("check_start_weight").
			Default(10),
		field.Bool("active").
			Default(true),
		field.String("mask").
			Default(""),
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

// Edges of the IdentifierSystem.
func (IdentifierSystem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("identifiers", PatientIdentifier.Type),
	}
}
