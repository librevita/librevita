package model

import (
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/pkg/ident"
)

// Standard system role names.
const (
	RoleAdmin        = "admin"
	RolePhysician    = "physician"
	RoleReceptionist = "receptionist"
	RolePatient      = "patient"
)

// Domain errors.
var (
	ErrUserNotFound       = errors.New("user: user not found")
	ErrDuplicateEmail     = errors.New("user: duplicate email")
	ErrDuplicateSpecialty = errors.New("user: specialty already exists")
	ErrSpecialtyScope     = errors.New("user: specialty does not belong to this clinic")
	ErrRoleNotFound       = errors.New("user: role not found")
	ErrRoleImmutable      = errors.New("user: cannot modify system role")
	ErrDuplicateRole      = errors.New("user: role already exists")
	ErrSystemRole         = errors.New("user: system roles cannot be renamed or deleted")
	ErrRoleInUse          = errors.New("user: cannot delete role with active users")
	ErrRequestNotFound    = errors.New("user: staff change request not found")
	ErrRequestProcessed   = errors.New("user: staff change request already processed")
	ErrRequestNotPending  = errors.New("user: staff change request is not pending")
	ErrAlreadyOnboarded   = errors.New("user: system is already onboarded")
	ErrInvalidCredentials = errors.New("user: invalid email or password")
	ErrEmailTaken         = errors.New("user: email is already registered")
	ErrEmailInUse         = errors.New("user: email is already in use")
	ErrCannotDemoteSelf   = errors.New("user: cannot change your own role or status")
	ErrLastActiveAdmin    = errors.New("user: cannot deactivate or demote the last active admin")
)

// User represents a user account domain model.
type User struct {
	ID           ident.UserID
	ClinicID     ident.ClinicID
	Email        string
	PasswordHash string
	DisplayName  string
	RoleID       ident.RoleID
	RoleName     string
	Active       bool
	Timezone     string
	UITheme      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Role represents a role domain model.
type Role struct {
	ID         ident.RoleID
	Name       string
	System     bool
	IsClinical bool
	CreatedAt  time.Time
}

// Specialty represents a medical specialty domain model.
type Specialty struct {
	ID        ident.SpecialtyID
	ClinicID  ident.ClinicID
	Name      string
	CreatedAt time.Time
}

// StaffChangeRequest represents a staff change request domain model.
type StaffChangeRequest struct {
	ID           ident.StaffChangeRequestID
	UserID       ident.UserID
	RequestedBy  ident.UserID
	Status       string
	Changes      string
	DecisionNote *string
	CreatedAt    time.Time
	DecidedAt    *time.Time
	DecidedBy    *ident.UserID
}

// Preferences holds user visual/locale preferences.
type Preferences struct {
	Timezone string
	UITheme  string
}

// Projections / Query Rows

// ListUsersRow is a projection for the users management list.
type ListUsersRow struct {
	ID          ident.UserID
	Email       string
	DisplayName string
	RoleName    string
	Active      bool
	CreatedAt   time.Time
}

// GetUserByIDRow is a projection for user detail views.
type GetUserByIDRow struct {
	ID             ident.UserID
	Email          string
	PasswordHash   string
	DisplayName    string
	RoleID         ident.RoleID
	RoleName       string
	RoleIsClinical bool
	Active         bool
	Timezone       string
	UITheme        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ListRecentUsersRow is a projection for the recent users widget.
type ListRecentUsersRow struct {
	ID          ident.UserID
	Email       string
	DisplayName string
	RoleName    string
	CreatedAt   time.Time
}

// ListPhysiciansPageRow is a projection for the physician directory table.
type ListPhysiciansPageRow struct {
	ID          ident.UserID
	DisplayName string
	Email       string
	Active      bool
	Specialties string
}

// ListStaffChangeRequestsByRequesterRow is a projection for requester's change history.
type ListStaffChangeRequestsByRequesterRow struct {
	ID           ident.StaffChangeRequestID
	UserID       ident.UserID
	UserName     string
	UserEmail    string
	StaffName    string
	StaffEmail   string
	RequestedBy  ident.UserID
	Status       string
	Changes      string
	DecisionNote *string
	CreatedAt    time.Time
	DecidedAt    *time.Time
}

// ListStaffChangeRequestsFilteredRow is a projection for admin change requests review.
type ListStaffChangeRequestsFilteredRow struct {
	ID            ident.StaffChangeRequestID
	UserID        ident.UserID
	StaffName     string
	StaffEmail    string
	RequestedBy   ident.UserID
	RequesterName string
	Status        string
	Changes       string
	DecisionNote  *string
	CreatedAt     time.Time
	DecidedAt     *time.Time
	DecidedBy     *ident.UserID
	DeciderName   *string
}
