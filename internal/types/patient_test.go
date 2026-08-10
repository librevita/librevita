package types

import "testing"

// TestPatientEnums mirrors the CHECK constraints on patients.sex and
// patients.status: only the schema options are valid, and the stored
// representation round-trips.
func TestPatientEnums(t *testing.T) {
	for _, sex := range []Sex{SexFemale, SexMale, SexOther, SexUnknown} {
		if !sex.Valid() {
			t.Fatalf("%q must be valid", sex)
		}
		if got, ok := ParseSex(sex.String()); !ok || got != sex {
			t.Fatalf("ParseSex(%q) = %q, %v; want %q, true", sex.String(), got, ok, sex)
		}
	}
	if Sex("nonbinary").Valid() {
		t.Fatal("sex outside the CHECK set must not be valid")
	}
	if _, ok := ParseSex("nonbinary"); ok {
		t.Fatal("ParseSex must reject values outside the CHECK set")
	}

	for _, status := range []PatientStatus{PatientStatusActive, PatientStatusInactive} {
		if !status.Valid() {
			t.Fatalf("%q must be valid", status)
		}
		if got, ok := ParsePatientStatus(status.String()); !ok || got != status {
			t.Fatalf("ParsePatientStatus(%q) = %q, %v; want %q, true", status.String(), got, ok, status)
		}
	}
	if PatientStatus("archived").Valid() {
		t.Fatal("status outside the CHECK set must not be valid")
	}
	if _, ok := ParsePatientStatus("archived"); ok {
		t.Fatal("ParsePatientStatus must reject values outside the CHECK set")
	}
}
