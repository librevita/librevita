package mixin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/database/schema/mixin"
	"librevita.org/pkg/ident"
)

func TestAllMixins(t *testing.T) {
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
