package model

import (
	"context"

	"github.com/google/uuid"

	clinicmodel "librevita.org/internal/domain/clinic/model"
)

// UserRepository defines the persistence contract for user accounts.
type UserRepository interface {
	Create(ctx context.Context, u *User) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*GetUserByIDRow, error)
	GetByEmail(ctx context.Context, email string) (*GetUserByIDRow, error)
	Update(ctx context.Context, u *User) (*User, error)
	UpdatePreferences(ctx context.Context, id uuid.UUID, timezone, theme string) error
	Count(ctx context.Context) (int64, error)
	CountByRole(ctx context.Context, roleName string) (int64, error)
	CountStaff(ctx context.Context, roleNames []string) (int64, error)
	CountActiveAdmins(ctx context.Context) (int, error)
	ListRecent(ctx context.Context, limit int) ([]ListRecentUsersRow, error)
	ListPage(ctx context.Context, query string, limit, offset int) ([]ListUsersRow, int64, error)
	ListPhysiciansPage(ctx context.Context, limit, offset int) ([]ListPhysiciansPageRow, int64, error)
	SetSpecialties(ctx context.Context, userID uuid.UUID, specialtyIDs []uuid.UUID) error
	ApplyApprovedStaffChange(ctx context.Context, reqID, userID, deciderID uuid.UUID, name, email string, specialtyIDs []uuid.UUID) error
}

// RoleRepository defines the persistence contract for system and custom roles.
type RoleRepository interface {
	List(ctx context.Context) ([]Role, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Role, error)
	GetByName(ctx context.Context, name string) (*Role, error)
	Create(ctx context.Context, r *Role) (*Role, error)
	Update(ctx context.Context, r *Role) (*Role, error)
	Delete(ctx context.Context, id uuid.UUID) error
	CountUsersWithRole(ctx context.Context, roleID uuid.UUID) (int, error)
	SeedDefaults(ctx context.Context) error
}

// SpecialtyRepository defines the persistence contract for medical specialties.
type SpecialtyRepository interface {
	ListByClinic(ctx context.Context, clinicID uuid.UUID) ([]Specialty, error)
	ListPageByClinic(ctx context.Context, clinicID uuid.UUID, limit, offset int) ([]Specialty, int64, error)
	Create(ctx context.Context, sp *Specialty) (*Specialty, error)
	Delete(ctx context.Context, clinicID, id uuid.UUID) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Specialty, error)
	CheckClinicScope(ctx context.Context, clinicID uuid.UUID, specialtyIDs []uuid.UUID) (bool, error)
}

// StaffRequestRepository defines the persistence contract for staff change requests.
type StaffRequestRepository interface {
	Create(ctx context.Context, req *StaffChangeRequest) (*StaffChangeRequest, error)
	GetByID(ctx context.Context, id uuid.UUID) (*StaffChangeRequest, error)
	ListByRequester(ctx context.Context, requesterID uuid.UUID, limit int) ([]ListStaffChangeRequestsByRequesterRow, error)
	ListFiltered(ctx context.Context, status, q string, limit, offset int) ([]ListStaffChangeRequestsFilteredRow, int64, error)
	Reject(ctx context.Context, id, deciderID uuid.UUID, note string) error
}

// SetupRepository defines the persistence contract for initial clinic onboarding.
type SetupRepository interface {
	IsOnboarded(ctx context.Context) (bool, error)
	Onboard(ctx context.Context, admin *User, clinic *clinicmodel.Clinic) (*User, error)
}
