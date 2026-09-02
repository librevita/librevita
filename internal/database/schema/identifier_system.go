package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

// IdentifierSystem holds the schema definition for the IdentifierSystem catalog.
type IdentifierSystem struct {
	ent.Schema
}

// Mixin of the IdentifierSystem.
func (IdentifierSystem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID[ident.IdentifierSystemID]{},
		mixin.Time{},
	}
}

// Fields of the IdentifierSystem.
func (IdentifierSystem) Fields() []ent.Field {
	return []ent.Field{
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
		field.Enum("transform").
			Values("none", "digits", "upper", "lower").
			Default("none").
			Comment("Canonicalization transform applied to raw input"),
		field.Enum("check_algorithm").
			Values("none", "mod11_desc", "mod11_cyclic").
			Default("none").
			Comment("Check-digit algorithm"),
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
		field.UUID("created_by", ident.UserID{}).
			Optional().
			Nillable(),
	}
}

// Edges of the IdentifierSystem.
func (IdentifierSystem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("identifiers", PatientIdentifier.Type),
		edge.To("clinic_opt_ins", ClinicIdentifierSystem.Type),
	}
}
