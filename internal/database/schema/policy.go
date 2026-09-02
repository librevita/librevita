package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
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

// Mixin of the AccessPolicy.
func (AccessPolicy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID[ident.PolicyID]{},
		mixin.Clinic{},
	}
}

// Fields of the AccessPolicy.
func (AccessPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("Permission name unique per clinic: dashboard.view, patient.edit, etc."),
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
		edge.From("clinic", Clinic.Type).
			Ref("policies").
			Field("clinic_id").
			Unique().
			Required(),
		edge.To("versions", AccessPolicyVersion.Type),
	}
}

// Indexes of the AccessPolicy.
func (AccessPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "name").
			Unique(),
	}
}
