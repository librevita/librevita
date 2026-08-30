package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

// ClinicalChild defines common patient and episode foreign keys and query indexes for SOAP clinical sub-resources.
type ClinicalChild struct {
	mixin.Schema
}

// Fields of the ClinicalChild mixin.
func (ClinicalChild) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("patient_id", uuid.UUID{}).
			Comment("Patient ID (denormalized for FLE / isolation)"),
		field.UUID("episode_id", uuid.UUID{}).
			Comment("Owning SOAP episode"),
	}
}

// Indexes of the ClinicalChild mixin.
func (ClinicalChild) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id", "episode_id"),
		index.Fields("patient_id", "created_at"),
		index.Fields("episode_id"),
	}
}
