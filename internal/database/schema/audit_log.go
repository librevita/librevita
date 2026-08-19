package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog holds the schema definition for the immutable AuditLog entity.
type AuditLog struct {
	ent.Schema
}

// Annotations of the AuditLog.
func (AuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "audit_log"},
	}
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Immutable(),
		field.String("actor_id").
			Optional().
			Nillable().
			Comment("UUID of actor; empty when anonymous"),
		field.String("actor_email").
			Optional().
			Nillable(),
		field.String("action").
			NotEmpty().
			Comment("e.g. register, login, logout, authorize"),
		field.String("resource").
			NotEmpty().
			Comment("e.g. user, session, patient"),
		field.Enum("result").
			Values("success", "failure").
			Comment("Outcome of the audited operation"),
		field.String("ip").
			Optional().
			Nillable(),
		field.String("request_id").
			Optional().
			Nillable(),
		field.String("detail").
			Optional().
			Nillable(),
		field.String("actor_name").
			Default(""),
		field.String("actor_role").
			Default(""),
		field.String("user_agent").
			Default(""),
		field.String("resource_name").
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.String("signature").
			NotEmpty().
			Comment("BLAKE2b cryptographic chain hash"),
	}
}

// Indexes of the AuditLog.
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("actor_id"),
		index.Fields("action"),
		index.Fields("created_at"),
		index.Fields("resource", "id"),
	}
}
