package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// Session defines PASETO token identification and expiration for session entities.
type Session struct {
	mixin.Schema
}

// Fields of the Session mixin.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("token_hash").
			NotEmpty().
			Immutable().
			Comment("Keyed BLAKE2b-256 fingerprint of the PASETO v4.local token id (jti)"),
		field.Time("expires_at").
			Comment("Expiration timestamp"),
	}
}

// Indexes of the Session mixin.
func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at"),
	}
}
