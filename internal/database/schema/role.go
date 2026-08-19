package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Role holds the schema definition for the Role entity.
type Role struct {
	ent.Schema
}

// Fields of the Role.
func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.String("name").
			NotEmpty().
			Unique().
			Comment("Unique role name: admin, physician, receptionist, patient"),
		field.Bool("system").
			Default(false).
			Comment("System roles cannot be modified or deleted"),
		field.Bool("is_clinical").
			Default(false).
			Comment("Indicates if role performs clinical actions"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the Role.
func (Role) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}
