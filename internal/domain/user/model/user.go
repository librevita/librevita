package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
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
	ID           uuid.UUID
	ClinicID     uuid.UUID
	Email        string
	PasswordHash string
	DisplayName  string
	RoleID       uuid.UUID
	RoleName     string
	Active       bool
	Timezone     string
	UITheme      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Role represents a role domain model.
type Role struct {
	ID         uuid.UUID
	Name       string
	System     bool
	IsClinical bool
	CreatedAt  time.Time
}

// Specialty represents a medical specialty domain model.
type Specialty struct {
	ID        uuid.UUID
	ClinicID  uuid.UUID
	Name      string
	CreatedAt time.Time
}

// StaffChangeRequest represents a staff change request domain model.
type StaffChangeRequest struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	RequestedBy  uuid.UUID
	Status       string
	Changes      string
	DecisionNote *string
	CreatedAt    time.Time
	DecidedAt    *time.Time
	DecidedBy    *uuid.UUID
}

// Preferences holds user visual/locale preferences.
type Preferences struct {
	Timezone string
	UITheme  string
}

// Projections / Query Rows

// ListUsersRow is a projection for the users management list.
type ListUsersRow struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	RoleName    string
	Active      bool
	CreatedAt   time.Time
}

// GetUserByIDRow is a projection for user detail views.
type GetUserByIDRow struct {
	ID             uuid.UUID
	Email          string
	PasswordHash   string
	DisplayName    string
	RoleID         uuid.UUID
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
	ID          uuid.UUID
	Email       string
	DisplayName string
	RoleName    string
	CreatedAt   time.Time
}

// ListPhysiciansPageRow is a projection for the physician directory table.
type ListPhysiciansPageRow struct {
	ID          uuid.UUID
	DisplayName string
	Email       string
	Active      bool
	Specialties string
}

// ListStaffChangeRequestsByRequesterRow is a projection for requester's change history.
type ListStaffChangeRequestsByRequesterRow struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	UserName     string
	UserEmail    string
	StaffName    string
	StaffEmail   string
	RequestedBy  uuid.UUID
	Status       string
	Changes      string
	DecisionNote *string
	CreatedAt    time.Time
	DecidedAt    *time.Time
}

// ListStaffChangeRequestsFilteredRow is a projection for admin change requests review.
type ListStaffChangeRequestsFilteredRow struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	StaffName     string
	StaffEmail    string
	RequestedBy   uuid.UUID
	RequesterName string
	Status        string
	Changes       string
	DecisionNote  *string
	CreatedAt     time.Time
	DecidedAt     *time.Time
	DecidedBy     *uuid.UUID
	DeciderName   *string
}
