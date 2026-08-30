package schema

import (
	"regexp"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"librevita.org/internal/database/schema/mixin"
)

// clinicSlugRE is the DNS-safe hostname label used as the clinic subdomain.
var clinicSlugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Clinic holds the schema definition for the Clinic / Tenant entity.
type Clinic struct {
	ent.Schema
}

// Mixin of the Clinic.
func (Clinic) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Time{},
	}
}

// Fields of the Clinic.
func (Clinic) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").
			NotEmpty().
			Unique().
			Immutable().
			MaxLen(63).
			Match(clinicSlugRE).
			Comment("DNS-safe subdomain label; immutable after creation"),
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
		field.Time("onboarded_at").
			Optional().
			Nillable().
			Comment("Set when clinic /setup completes; nil means a provisioned shell"),
	}
}

// Edges of the Clinic.
func (Clinic) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
		edge.To("roles", Role.Type),
		edge.To("policies", AccessPolicy.Type),
		edge.To("patients", Patient.Type),
		edge.To("specialties", Specialty.Type),
		edge.To("appointments", Appointment.Type),
		edge.To("episodes", Episode.Type),
		edge.To("findings", Finding.Type),
		edge.To("problems", Problem.Type),
		edge.To("plan_items", PlanItem.Type),
		edge.To("identifier_systems", ClinicIdentifierSystem.Type),
		edge.To("patient_identifiers", PatientIdentifier.Type),
		edge.To("staff_requests", StaffChangeRequest.Type),
		edge.To("storage_objects", StorageObject.Type),
		edge.To("audit_logs", AuditLog.Type),
	}
}
