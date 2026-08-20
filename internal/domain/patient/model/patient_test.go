package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/domain/patient/model"
)

// TestPatientEnums mirrors the CHECK constraints on patients.sex and
// patients.status: only the schema options are valid, and the stored
// representation round-trips.
func TestPatientEnums(t *testing.T) {
	for _, sex := range []model.Sex{model.SexFemale, model.SexMale, model.SexOther, model.SexUnknown} {
		assert.True(t, sex.Valid(), "%q must be valid", sex)
		got, ok := model.ParseSex(sex.String())
		assert.True(t, ok)
		assert.Equal(t, sex, got)
	}
	assert.False(t, model.Sex("nonbinary").Valid())
	_, ok := model.ParseSex("nonbinary")
	assert.False(t, ok)

	for _, status := range []model.PatientStatus{model.PatientStatusActive, model.PatientStatusInactive, model.PatientStatusArchived} {
		assert.True(t, status.Valid(), "%q must be valid", status)
		got, ok := model.ParsePatientStatus(status.String())
		assert.True(t, ok)
		assert.Equal(t, status, got)
	}
	assert.False(t, model.PatientStatus("suspended").Valid())
	_, ok = model.ParsePatientStatus("suspended")
	assert.False(t, ok)
}
