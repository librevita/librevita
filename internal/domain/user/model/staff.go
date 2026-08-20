package model

// StaffRequestStatus is the state of a staff change request. It mirrors
// the CHECK constraint on staff_change_requests.status (see
// db/migrations/00008_staff_requests.sql).
type StaffRequestStatus string

const (
	StaffRequestPending  StaffRequestStatus = "pending"
	StaffRequestApproved StaffRequestStatus = "approved"
	StaffRequestRejected StaffRequestStatus = "rejected"
)

// Valid reports whether s is one of the options the database CHECK
// constraint accepts.
func (s StaffRequestStatus) Valid() bool {
	switch s {
	case StaffRequestPending, StaffRequestApproved, StaffRequestRejected:
		return true
	}
	return false
}

// String returns the stored representation of s.
func (s StaffRequestStatus) String() string {
	return string(s)
}

// ParseStaffRequestStatus converts a stored value back to the enum. ok
// is false when the value is not one of the CHECK options.
func ParseStaffRequestStatus(s string) (StaffRequestStatus, bool) {
	status := StaffRequestStatus(s)
	return status, status.Valid()
}
