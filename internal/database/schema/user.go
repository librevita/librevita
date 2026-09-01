package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Mixin of the User.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.Time{},
	}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			NotEmpty().
			Comment("Email unique per clinic (clinic_id, email)"),
		field.String("password_hash").
			NotEmpty().
			Sensitive().
			Comment("Argon2id password hash"),
		field.String("display_name").
			NotEmpty().
			Comment("User full display name"),
		field.UUID("role_id", uuid.UUID{}).
			Comment("Assigned Role ID"),
		field.Bool("active").
			Default(true).
			Comment("Whether user account is active"),
		field.String("timezone").
			Default("").
			Comment("User personal IANA timezone"),
		field.Enum("ui_theme").
			Values("system", "light", "dark").
			Default("system").
			Comment("User UI theme preference"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("users").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("role", Role.Type).
			Ref("users").
			Field("role_id").
			Unique().
			Required(),
		edge.To("specialties", Specialty.Type),
		edge.To("staff_requests", StaffChangeRequest.Type),
		edge.To("appointments", Appointment.Type),
		edge.To("episodes", Episode.Type),
		edge.To("portal_patient", Patient.Type),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "email").
			Unique(),
	}
}
