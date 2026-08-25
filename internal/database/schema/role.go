package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
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
		field.UUID("clinic_id", uuid.UUID{}).
			Comment("Owning clinic; system roles are copied at clinic onboard"),
		field.String("name").
			NotEmpty().
			Comment("Role name unique per clinic: admin, physician, receptionist, patient"),
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
		edge.From("clinic", Clinic.Type).
			Ref("roles").
			Field("clinic_id").
			Unique().
			Required(),
		edge.To("users", User.Type),
	}
}

// Indexes of the Role.
func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "name").
			Unique(),
	}
}
