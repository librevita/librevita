package urn_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"librevita.org/pkg/urn"
)

func TestConstructors(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000d1")

	assert.Equal(t, "urn:librevita:clinic:"+clinicID.String(), urn.Clinic(clinicID))
	assert.Equal(t, "urn:librevita:clinic:"+clinicID.String()+":patient:"+patientID.String(),
		urn.Patient(clinicID, patientID))
	assert.Equal(t, "urn:librevita:meta:setup_completed", urn.Meta("setup_completed"))
	assert.Equal(t, "urn:librevita:clinic:"+clinicID.String()+":session:blake2s$abc",
		urn.ClinicSession(clinicID, "blake2s$abc"))
	assert.Equal(t, "urn:librevita:platform:session:blake2s$abc",
		urn.PlatformSession("blake2s$abc"))
	assert.Equal(t, "urn:librevita:id:br:cpf", urn.Identifier("br", "cpf"))
	assert.Equal(t, "urn:librevita:id:passport", urn.Identifier("passport"))
	assert.Equal(t, "urn:librevita:id:raw", urn.IdentifierRaw)
	assert.Equal(t, urn.Identifier("raw"), urn.IdentifierRaw)
}

func TestParsePatient(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	patientID := uuid.MustParse("01990000-0000-7000-8000-0000000000d1")
	gotClinic, gotPatient, ok := urn.ParsePatient(urn.Patient(clinicID, patientID))
	assert.True(t, ok)
	assert.Equal(t, clinicID, gotClinic)
	assert.Equal(t, patientID, gotPatient)

	_, _, ok = urn.ParsePatient(urn.Clinic(clinicID))
	assert.False(t, ok)
	_, _, ok = urn.ParsePatient(urn.ClinicSession(clinicID, "blake2s$abc"))
	assert.False(t, ok)
	_, _, ok = urn.ParsePatient("not-a-urn")
	assert.False(t, ok)
}

func TestParseClinic(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	got, ok := urn.ParseClinic(urn.Clinic(clinicID))
	assert.True(t, ok)
	assert.Equal(t, clinicID, got)

	_, ok = urn.ParseClinic(urn.Patient(clinicID, uuid.MustParse("01990000-0000-7000-8000-0000000000d1")))
	assert.False(t, ok)
	_, ok = urn.ParseClinic(urn.ClinicSession(clinicID, "blake2s$abc"))
	assert.False(t, ok)
	_, ok = urn.ParseClinic("urn:librevita:clinic:")
	assert.False(t, ok)
}

func TestParseIdentifier(t *testing.T) {
	got, ok := urn.ParseIdentifier(urn.Identifier("br", "cpf"))
	assert.True(t, ok)
	assert.Equal(t, []string{"br", "cpf"}, got)

	got, ok = urn.ParseIdentifier(urn.Identifier("passport"))
	assert.True(t, ok)
	assert.Equal(t, []string{"passport"}, got)

	got, ok = urn.ParseIdentifier(urn.IdentifierRaw)
	assert.True(t, ok)
	assert.Equal(t, []string{"raw"}, got)

	_, ok = urn.ParseIdentifier(urn.IdentifierPrefix)
	assert.False(t, ok)
	_, ok = urn.ParseIdentifier(urn.IdentifierPrefix + "br:")
	assert.False(t, ok)
	_, ok = urn.ParseIdentifier(urn.Clinic(uuid.MustParse("01990000-0000-7000-8000-0000000000c1")))
	assert.False(t, ok)
}
