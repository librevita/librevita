package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// PlatformSession is a PASETO session bound to a platform_users row.
type PlatformSession struct {
	ent.Schema
}

// Annotations of the PlatformSession.
func (PlatformSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "platform_sessions"},
	}
}

// Mixin of the PlatformSession.
func (PlatformSession) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Session{},
	}
}

// Fields of the PlatformSession.
func (PlatformSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("platform_user_id", uuid.UUID{}).
			Comment("Authenticated platform operator"),
	}
}

// Edges of the PlatformSession.
func (PlatformSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", PlatformUser.Type).
			Ref("sessions").
			Field("platform_user_id").
			Unique().
			Required(),
	}
}

// Indexes of the PlatformSession.
func (PlatformSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform_user_id"),
	}
}
