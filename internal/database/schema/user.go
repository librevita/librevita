package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("email").
			NotEmpty().
			Unique().
			Comment("Unique email address for authentication"),
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
		field.String("ui_theme").
			Default("system").
			Comment("User UI theme preference: system, light, dark"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("role", Role.Type).
			Ref("users").
			Field("role_id").
			Unique().
			Required(),
		edge.To("sessions", Session.Type),
		edge.To("specialties", Specialty.Type),
		edge.To("staff_requests", StaffChangeRequest.Type),
		edge.To("appointments", Appointment.Type),
		edge.To("episodes", Episode.Type),
	}
}
