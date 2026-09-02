package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

// PlanItem holds a structured plan activity belonging to an Episode.
type PlanItem struct {
	ent.Schema
}

// Mixin of the PlanItem.
func (PlanItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID[ident.PlanItemID]{},
		mixin.Clinic{},
		mixin.ClinicalChild{},
		mixin.Time{},
	}
}

// Fields of the PlanItem.
func (PlanItem) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("kind").
			Values("medication", "procedure", "exam", "appointment", "instruction").
			Default("instruction"),
		field.Enum("status").
			Values("draft", "active", "completed", "cancelled").
			Default("active"),
		field.Time("scheduled_at").
			Optional().
			Nillable(),

		field.String("code_system").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity code system (PHI)"),
		field.String("code").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity code (PHI)"),
		field.String("display").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Activity display (PHI)"),
		field.String("description").
			SchemaType(blobType).
			ValueScanner(fle.EncryptedString()).
			Optional().
			Comment("Plan narrative (PHI)"),
	}
}

// Edges of the PlanItem.
func (PlanItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("plan_items").
			Field("clinic_id").
			Unique().
			Required(),
		edge.From("patient", Patient.Type).
			Ref("plan_items").
			Field("patient_id").
			Unique().
			Required(),
		edge.From("episode", Episode.Type).
			Ref("plan_items").
			Field("episode_id").
			Unique().
			Required(),
	}
}
