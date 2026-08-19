package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// ZeroKnowledgeMixin embeds blind index and encrypted payload fields into an Ent schema.
// Entities adopting this mixin store no cleartext sensitive information in database columns.
type ZeroKnowledgeMixin struct {
	mixin.Schema
}

// Fields of the ZeroKnowledgeMixin.
func (ZeroKnowledgeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.String("blind_index").
			MaxLen(64).
			NotEmpty().
			Comment("BLAKE2b-256 keyed hash for exact-match searches without revealing cleartext"),
		field.Bytes("encrypted_payload").
			NotEmpty().
			Comment("XChaCha20-Poly1305 encrypted domain payload"),
		field.Bytes("nonce").
			MaxLen(24).
			NotEmpty().
			Comment("24-byte cryptographic nonce for XChaCha20-Poly1305"),
	}
}

// Indexes of the ZeroKnowledgeMixin.
func (ZeroKnowledgeMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("blind_index"),
	}
}
