package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/database/schema"
	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

func TestSchemas(t *testing.T) {
	// Appointment
	{
		s := schema.Appointment{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// AuditLog
	{
		s := schema.AuditLog{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// Clinic
	{
		s := schema.Clinic{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
		_ = s.Annotations()
	}

	// ClinicIdentifierSystem
	{
		s := schema.ClinicIdentifierSystem{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// Episode
	{
		s := schema.Episode{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// Finding
	{
		s := schema.Finding{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// IdentifierSystem
	{
		s := schema.IdentifierSystem{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// Patient
	{
		s := schema.Patient{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
		_ = s.Annotations()
	}

	// PatientIdentifier
	{
		s := schema.PatientIdentifier{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// PlanItem
	{
		s := schema.PlanItem{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// PlatformUser
	{
		s := schema.PlatformUser{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		_ = s.Edges()
	}

	// AccessPolicy
	{
		s := schema.AccessPolicy{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
		_ = s.Annotations()
	}

	// AccessPolicyVersion
	{
		s := schema.AccessPolicyVersion{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
		_ = s.Annotations()
	}

	// Problem
	{
		s := schema.Problem{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
	}

	// Role
	{
		s := schema.Role{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// Specialty
	{
		s := schema.Specialty{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// StaffChangeRequest
	{
		s := schema.StaffChangeRequest{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// StorageObject
	{
		s := schema.StorageObject{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		assert.NotNil(t, s.Indexes())
		assert.NotNil(t, s.Edges())
	}

	// User
	{
		s := schema.User{}
		assert.NotNil(t, s.Fields())
		assert.NotNil(t, s.Mixin())
		_ = s.Indexes()
		assert.NotNil(t, s.Edges())
		_ = s.Annotations()
	}
}

func TestMixins(t *testing.T) {
	// Clinic Mixin
	{
		m := mixin.Clinic{}
		assert.NotEmpty(t, m.Fields())
	}

	// ClinicalChild Mixin
	{
		m := mixin.ClinicalChild{}
		assert.NotEmpty(t, m.Fields())
		assert.NotEmpty(t, m.Indexes())
	}

	// CreatedAt Mixin
	{
		m := mixin.CreatedAt{}
		assert.NotEmpty(t, m.Fields())
	}

	// Time Mixin
	{
		m := mixin.Time{}
		assert.NotEmpty(t, m.Fields())
		_ = m.Indexes()
	}

	// UUID Mixin
	{
		m := mixin.UUID[ident.PatientID]{}
		assert.NotEmpty(t, m.Fields())
	}
}
