package http

import (
	"testing"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
)

func TestPatientChanges(t *testing.T) {
	before := &repository.GetPatientWithCreatorRow{
		DisplayName: "Ana Souza",
		Sex:         "female",
		Phone:       strPtr("11"),
		Email:       strPtr("ana@t.com"),
		City:        nil,
	}
	cases := []struct {
		name  string
		input usecase.PatientInput
		want  string
	}{
		{
			name:  "no changes",
			input: usecase.PatientInput{DisplayName: "Ana Souza", Sex: "female", Phone: "11", Email: "ana@t.com"},
			want:  "",
		},
		{
			name:  "changed values",
			input: usecase.PatientInput{DisplayName: "Ana Souza Silva", Sex: "female", Phone: "22", Email: "ana@t.com"},
			want:  "display name: Ana Souza -> Ana Souza Silva, phone: 11 -> 22",
		},
		{
			name:  "new value",
			input: usecase.PatientInput{DisplayName: "Ana Souza", Sex: "female", Phone: "11", Email: "ana@t.com", City: "Sao Paulo"},
			want:  "city: Sao Paulo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patientChanges(before, tc.input); got != tc.want {
				t.Errorf("patientChanges = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDisplayValue(t *testing.T) {
	short := "abc"
	if got := displayValue(short); got != short {
		t.Errorf("displayValue(%q) = %q, want unchanged", short, got)
	}
	long := "0123456789012345678901234567890123456789X"
	if got, want := displayValue(long), "0123456789012345678901234567890123456..."; got != want {
		t.Errorf("displayValue(long) = %q, want %q", got, want)
	}
}

func TestHistoryText(t *testing.T) {
	email := "nurse@clinic.org"
	detail := "phone: 11 -> 22"
	cases := []struct {
		name string
		ev   audit.EventRow
		want string
	}{
		{
			name: "create",
			ev:   audit.EventRow{Action: "patient.create", ActorEmail: &email},
			want: "Registered by nurse@clinic.org",
		},
		{
			name: "update with changes",
			ev:   audit.EventRow{Action: "patient.update", ActorEmail: &email, Detail: &detail},
			want: "Updated by nurse@clinic.org (phone: 11 -> 22)",
		},
		{
			name: "update without detail",
			ev:   audit.EventRow{Action: "patient.update", ActorEmail: &email},
			want: "Updated by nurse@clinic.org",
		},
		{
			name: "archived",
			ev:   audit.EventRow{Action: "patient.status", ActorEmail: &email, Detail: strPtr("inactive")},
			want: "Archived by nurse@clinic.org",
		},
		{
			name: "restored",
			ev:   audit.EventRow{Action: "patient.status", ActorEmail: &email, Detail: strPtr("active")},
			want: "Restored by nurse@clinic.org",
		},
		{
			name: "unknown actor",
			ev:   audit.EventRow{Action: "patient.status", Detail: strPtr("inactive")},
			want: "Archived by an unknown user",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyText(tc.ev); got != tc.want {
				t.Errorf("historyText = %q, want %q", got, tc.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
