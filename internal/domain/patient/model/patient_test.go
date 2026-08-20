package model_test

import (
	"testing"

	"librevita.org/internal/domain/patient/model"
)

// TestPatientEnums mirrors the CHECK constraints on patients.sex and
// patients.status: only the schema options are valid, and the stored
// representation round-trips.
func TestPatientEnums(t *testing.T) {
	for _, sex := range []model.Sex{model.SexFemale, model.SexMale, model.SexOther, model.SexUnknown} {
		if !sex.Valid() {
			t.Fatalf("%q must be valid", sex)
		}
		if got, ok := model.ParseSex(sex.String()); !ok || got != sex {
			t.Fatalf("ParseSex(%q) = %q, %v; want %q, true", sex.String(), got, ok, sex)
		}
	}
	if model.Sex("nonbinary").Valid() {
		t.Fatal("sex outside the CHECK set must not be valid")
	}
	if _, ok := model.ParseSex("nonbinary"); ok {
		t.Fatal("ParseSex must reject values outside the CHECK set")
	}

	for _, status := range []model.PatientStatus{model.PatientStatusActive, model.PatientStatusInactive, model.PatientStatusArchived} {
		if !status.Valid() {
			t.Fatalf("%q must be valid", status)
		}
		if got, ok := model.ParsePatientStatus(status.String()); !ok || got != status {
			t.Fatalf("ParsePatientStatus(%q) = %q, %v; want %q, true", status.String(), got, ok, status)
		}
	}
	if model.PatientStatus("suspended").Valid() {
		t.Fatal("status outside the CHECK set must not be valid")
	}
	if _, ok := model.ParsePatientStatus("suspended"); ok {
		t.Fatal("ParsePatientStatus must reject values outside the CHECK set")
	}
}
