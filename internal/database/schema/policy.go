package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// AccessPolicy holds the schema definition for dynamic CEL authorization policies.
type AccessPolicy struct {
	ent.Schema
}

// Annotations of the AccessPolicy.
func (AccessPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "policies"},
	}
}

// Fields of the AccessPolicy.
func (AccessPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.String("name").
			NotEmpty().
			Unique().
			Comment("Permission name: dashboard.view, patient.edit, etc."),
		field.String("expression").
			NotEmpty().
			Comment("CEL expression"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the AccessPolicy.
func (AccessPolicy) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("versions", AccessPolicyVersion.Type),
	}
}
