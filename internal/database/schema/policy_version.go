package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

// AccessPolicyVersion holds the schema definition for historical policy expression snapshots.
type AccessPolicyVersion struct {
	ent.Schema
}

// Annotations of the AccessPolicyVersion.
func (AccessPolicyVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "policy_versions"},
	}
}

// Mixin of the AccessPolicyVersion.
func (AccessPolicyVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreatedAt{},
	}
}

// Fields of the AccessPolicyVersion.
func (AccessPolicyVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Immutable(),
		field.UUID("policy_id", ident.PolicyID{}).
			Comment("Parent Policy ID"),
		field.String("expression").
			NotEmpty().
			Comment("CEL expression historical snapshot"),
		field.String("changed_by").
			Optional().
			Nillable(),
		field.String("changed_by_email").
			Optional().
			Nillable(),
		field.Enum("origin").
			Values("seed", "admin", "system").
			Default("system").
			Comment("Origin of the policy version"),
	}
}

// Edges of the AccessPolicyVersion.
func (AccessPolicyVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("policy", AccessPolicy.Type).
			Ref("versions").
			Field("policy_id").
			Unique().
			Required(),
	}
}

// Indexes of the AccessPolicyVersion.
func (AccessPolicyVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("policy_id", "id"),
	}
}
