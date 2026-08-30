package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// StaffChangeRequest holds the schema definition for the StaffChangeRequest entity.
type StaffChangeRequest struct {
	ent.Schema
}

// Mixin of the StaffChangeRequest.
func (StaffChangeRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.CreatedAt{},
	}
}

// Fields of the StaffChangeRequest.
func (StaffChangeRequest) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}).
			Comment("Target staff user account ID"),
		field.UUID("requested_by", uuid.UUID{}).
			Comment("User ID who created the request"),
		field.Enum("status").
			Values("pending", "approved", "rejected").
			Default("pending").
			Comment("Request lifecycle status"),
		field.String("changes").
			NotEmpty().
			Comment("JSON payload describing requested attribute changes"),
		field.String("decision_note").
			Optional().
			Nillable().
			Comment("Note justifying the approval or rejection"),
		field.Time("decided_at").
			Optional().
			Nillable(),
		field.UUID("decided_by", uuid.UUID{}).
			Optional().
			Nillable(),
	}
}

// Edges of the StaffChangeRequest.
func (StaffChangeRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("staff_requests").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("staff_requests").
			Field("user_id").
			Unique().
			Required(),
		edge.To("requester", User.Type).
			Field("requested_by").
			Unique().
			Required(),
		edge.To("decider", User.Type).
			Field("decided_by").
			Unique(),
	}
}

// Indexes of the StaffChangeRequest.
func (StaffChangeRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id"),
		index.Fields("status", "created_at"),
		index.Fields("user_id"),
		index.Fields("requested_by", "created_at"),
	}
}
