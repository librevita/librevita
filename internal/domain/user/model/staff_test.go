package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/domain/user/model"
)

// TestStaffRequestStatus mirrors the CHECK constraint on
// staff_change_requests.status: only pending/approved/rejected are
// valid, and the stored representation round-trips.
func TestStaffRequestStatus(t *testing.T) {
	for _, status := range []model.StaffRequestStatus{model.StaffRequestPending, model.StaffRequestApproved, model.StaffRequestRejected} {
		assert.True(t, status.Valid(), "%q must be valid", status)
		got, ok := model.ParseStaffRequestStatus(status.String())
		assert.True(t, ok)
		assert.Equal(t, status, got)
	}
	assert.False(t, model.StaffRequestStatus("deleted").Valid())
	_, ok := model.ParseStaffRequestStatus("deleted")
	assert.False(t, ok)
}
