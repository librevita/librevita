package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Clinic holds the schema definition for the Clinic / Tenant entity.
type Clinic struct {
	ent.Schema
}

// Fields of the Clinic.
func (Clinic) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(newUUIDv7).
			Immutable(),
		field.String("name").
			NotEmpty().
			Comment("Clinic legal or trade name"),
		field.String("tax_id").
			Optional().
			Comment("CNPJ / NIF / Tax identification"),
		field.String("phone").
			Optional(),
		field.String("email").
			Optional(),
		field.String("street").
			Optional(),
		field.String("city").
			Optional(),
		field.String("state").
			Optional(),
		field.String("postal_code").
			Optional(),
		field.String("country").
			Default("BR"),
		field.String("timezone").
			Default("America/Sao_Paulo"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Clinic.
func (Clinic) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("patients", Patient.Type),
		edge.To("specialties", Specialty.Type),
		edge.To("appointments", Appointment.Type),
		edge.To("episodes", Episode.Type),
	}
}
