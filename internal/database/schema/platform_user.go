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

// Fields of the PlatformUser.
func (PlatformUser) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
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
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the PlatformUser.
func (PlatformUser) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("sessions", PlatformSession.Type),
	}
}
