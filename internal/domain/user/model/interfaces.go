package model

import (
	"context"

	"librevita.org/pkg/ident"
)

// UserRepository defines the persistence contract for user accounts.
type UserRepository interface {
	Create(ctx context.Context, u *User) (*User, error)
	BindPortalPatient(ctx context.Context, userID ident.UserID, patientID ident.PatientID) error
	GetByID(ctx context.Context, id ident.UserID) (*GetUserByIDRow, error)
	GetByEmail(ctx context.Context, email string) (*GetUserByIDRow, error)
	Update(ctx context.Context, u *User) (*User, error)
	UpdatePreferences(ctx context.Context, id ident.UserID, timezone, theme string) error
	Count(ctx context.Context) (int64, error)
	CountByRole(ctx context.Context, roleName string) (int64, error)
	CountStaff(ctx context.Context, roleNames []string) (int64, error)
	CountActiveAdmins(ctx context.Context) (int, error)
	ListRecent(ctx context.Context, limit int) ([]ListRecentUsersRow, error)
	ListPage(ctx context.Context, query string, limit, offset int) ([]ListUsersRow, int64, error)
	ListPhysiciansPage(ctx context.Context, limit, offset int) ([]ListPhysiciansPageRow, int64, error)
	SetSpecialties(ctx context.Context, userID ident.UserID, specialtyIDs []ident.SpecialtyID) error
	ApplyApprovedStaffChange(ctx context.Context, reqID ident.StaffChangeRequestID, userID, deciderID ident.UserID, name, email string, specialtyIDs []ident.SpecialtyID) error
}

// RoleRepository defines the persistence contract for system and custom roles.
type RoleRepository interface {
	List(ctx context.Context) ([]Role, error)
	GetByID(ctx context.Context, id ident.RoleID) (*Role, error)
	GetByName(ctx context.Context, name string) (*Role, error)
	Create(ctx context.Context, r *Role) (*Role, error)
	Update(ctx context.Context, r *Role) (*Role, error)
	Delete(ctx context.Context, id ident.RoleID) error
	CountUsersWithRole(ctx context.Context, roleID ident.RoleID) (int, error)
	SeedDefaults(ctx context.Context) error
}

// SpecialtyRepository defines the persistence contract for medical specialties.
type SpecialtyRepository interface {
	ListByClinic(ctx context.Context, clinicID ident.ClinicID) ([]Specialty, error)
	ListPageByClinic(ctx context.Context, clinicID ident.ClinicID, limit, offset int) ([]Specialty, int64, error)
	Create(ctx context.Context, sp *Specialty) (*Specialty, error)
	Delete(ctx context.Context, clinicID ident.ClinicID, id ident.SpecialtyID) error
	ListByUser(ctx context.Context, userID ident.UserID) ([]Specialty, error)
	CheckClinicScope(ctx context.Context, clinicID ident.ClinicID, specialtyIDs []ident.SpecialtyID) (bool, error)
}

// StaffRequestRepository defines the persistence contract for staff change requests.
type StaffRequestRepository interface {
	Create(ctx context.Context, req *StaffChangeRequest) (*StaffChangeRequest, error)
	GetByID(ctx context.Context, id ident.StaffChangeRequestID) (*StaffChangeRequest, error)
	ListByRequester(ctx context.Context, requesterID ident.UserID, limit int) ([]ListStaffChangeRequestsByRequesterRow, error)
	ListFiltered(ctx context.Context, status, q string, limit, offset int) ([]ListStaffChangeRequestsFilteredRow, int64, error)
	Reject(ctx context.Context, id ident.StaffChangeRequestID, deciderID ident.UserID, note string) error
}

// SetupRepository defines the persistence contract for initial clinic onboarding.
type SetupRepository interface {
	IsOnboarded(ctx context.Context) (bool, error)
	Onboard(ctx context.Context, admin *User, systemIDs []ident.IdentifierSystemID) (*User, error)
}
