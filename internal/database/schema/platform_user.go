package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/database/schema/mixin"
)

// PlatformUser is an installation operator. They authenticate only on the
// apex host and have no clinic_id.
type PlatformUser struct {
	ent.Schema
}

// Annotations of the PlatformUser.
func (PlatformUser) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "platform_users"},
	}
}

// Mixin of the PlatformUser.
func (PlatformUser) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Time{},
	}
}

// Fields of the PlatformUser.
func (PlatformUser) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			NotEmpty().
			Unique().
			Comment("Globally unique email for apex authentication"),
		field.String("password_hash").
			NotEmpty().
			Sensitive().
			Comment("Argon2id password hash"),
		field.String("display_name").
			NotEmpty(),
		field.Bool("active").
			Default(true),
	}
}

// Edges of the PlatformUser.
func (PlatformUser) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("sessions", PlatformSession.Type),
	}
}
