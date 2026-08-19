package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
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

// Fields of the AccessPolicyVersion.
func (AccessPolicyVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Immutable(),
		field.UUID("policy_id", uuid.UUID{}).
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
		field.String("origin").
			Default("system").
			Comment("Origin: seed, admin, system"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
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
