package model_test

import (
	"testing"

	"librevita.org/internal/domain/user/model"
)

// TestStaffRequestStatus mirrors the CHECK constraint on
// staff_change_requests.status: only pending/approved/rejected are
// valid, and the stored representation round-trips.
func TestStaffRequestStatus(t *testing.T) {
	for _, status := range []model.StaffRequestStatus{model.StaffRequestPending, model.StaffRequestApproved, model.StaffRequestRejected} {
		if !status.Valid() {
			t.Fatalf("%q must be valid", status)
		}
		if got, ok := model.ParseStaffRequestStatus(status.String()); !ok || got != status {
			t.Fatalf("ParseStaffRequestStatus(%q) = %q, %v; want %q, true", status.String(), got, ok, status)
		}
	}
	if model.StaffRequestStatus("deleted").Valid() {
		t.Fatal("status outside the CHECK set must not be valid")
	}
	if _, ok := model.ParseStaffRequestStatus("deleted"); ok {
		t.Fatal("ParseStaffRequestStatus must reject values outside the CHECK set")
	}
}
