package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Session holds the schema definition for the Session entity.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("token_hash").
			NotEmpty().
			Immutable().
			Comment("Keyed BLAKE2b-256 fingerprint of the PASETO v4.local token id (jti)"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Account ID of the authenticated user"),
		field.Time("expires_at").
			Comment("Expiration timestamp"),
	}
}

// Edges of the Session.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("sessions").
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the Session.
func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}
