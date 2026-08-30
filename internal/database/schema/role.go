package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"librevita.org/internal/database/schema/mixin"
)

// Role holds the schema definition for the Role entity.
type Role struct {
	ent.Schema
}

// Mixin of the Role.
func (Role) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.CreatedAt{},
	}
}

// Fields of the Role.
func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("Role name unique per clinic: admin, physician, receptionist, patient"),
		field.Bool("system").
			Default(false).
			Comment("System roles cannot be modified or deleted"),
		field.Bool("is_clinical").
			Default(false).
			Comment("Indicates if role performs clinical actions"),
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
