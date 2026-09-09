package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/domain/clinic/model"
	"librevita.org/pkg/ident"
)

func TestValidDomain(t *testing.T) {
	assert.True(t, model.ValidDomain("clinicasaojose.com.br"))
	assert.True(t, model.ValidDomain("clinic-alpha.org"))
	assert.True(t, model.ValidDomain("clinica123.local"))
	assert.True(t, model.ValidDomain("localhost"))
	assert.False(t, model.ValidDomain("INVALID!"))
	assert.False(t, model.ValidDomain("-invalid.com"))
	assert.False(t, model.ValidDomain(""))
}

func TestValidSlug(t *testing.T) {
	assert.True(t, model.ValidSlug("clinic-alpha.org"))
	assert.True(t, model.ValidSlug("clinica123"))
	assert.False(t, model.ValidSlug("INVALID!")) // regex fail
	assert.False(t, model.ValidSlug("-invalid")) // leading dash
}

func TestClinicOnboarded(t *testing.T) {
	var nilClinic *model.Clinic
	assert.False(t, nilClinic.Onboarded())

	c := &model.Clinic{
		ID:   ident.New[ident.ClinicID](),
		Slug: "test",
		Name: "Test",
	}
	assert.False(t, c.Onboarded())

	now := time.Now().UTC()
	c.OnboardedAt = &now
	assert.True(t, c.Onboarded())
}

func TestValidTimezone(t *testing.T) {
	assert.True(t, model.ValidTimezone("America/Sao_Paulo"))
	assert.True(t, model.ValidTimezone("UTC"))
	assert.False(t, model.ValidTimezone("Invalid/Tz"))

	assert.NotEmpty(t, model.TimeZones)
	for _, grp := range model.TimeZones {
		assert.NotEmpty(t, grp.Label)
		for _, tz := range grp.Zones {
			assert.True(t, model.ValidTimezone(tz))
		}
	}
}
