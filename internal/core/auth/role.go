// Role is an authorization level assigned to a user account.
package auth

import (
	"fmt"
)

// Role is an authorization level assigned to a user account.
type Role string

// Supported roles. The database CHECK constraint in db/migrations must stay
// in sync with this list.
const (
	RoleAdmin        Role = "admin"
	RolePhysician    Role = "physician"
	RoleReceptionist Role = "receptionist"
	RolePatient      Role = "patient"
)

// String returns the lowercase role name.
func (r Role) String() string { return string(r) }

// Valid reports whether r is a supported role.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RolePhysician, RoleReceptionist, RolePatient:
		return true
	default:
		return false
	}
}

// ParseRole converts a stored role value.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !r.Valid() {
		return "", fmt.Errorf("security: invalid role %q", s)
	}
	return r, nil
}
