package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
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

// Fields of the PlatformSession.
func (PlatformSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("token_hash").
			NotEmpty().
			Immutable().
			Comment("Keyed BLAKE2b-256 fingerprint of the PASETO v4.local token id (jti)"),
		field.UUID("platform_user_id", uuid.UUID{}).
			Comment("Authenticated platform operator"),
		field.Time("expires_at").
			Comment("Expiration timestamp"),
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
		index.Fields("expires_at"),
	}
}
