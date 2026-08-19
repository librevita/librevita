package usecase

import (
	usermodel "librevita.org/internal/domain/user/model"
)

// Re-export domain models and errors from user/model.
type (
	User                                  = usermodel.User
	Role                                  = usermodel.Role
	Specialty                             = usermodel.Specialty
	StaffChangeRequest                    = usermodel.StaffChangeRequest
	Preferences                           = usermodel.Preferences
	ListUsersRow                          = usermodel.ListUsersRow
	GetUserByIDRow                        = usermodel.GetUserByIDRow
	ListRecentUsersRow                    = usermodel.ListRecentUsersRow
	ListPhysiciansPageRow                 = usermodel.ListPhysiciansPageRow
	ListStaffChangeRequestsByRequesterRow = usermodel.ListStaffChangeRequestsByRequesterRow
	ListStaffChangeRequestsFilteredRow    = usermodel.ListStaffChangeRequestsFilteredRow
)

const (
	RoleAdmin        = usermodel.RoleAdmin
	RolePhysician    = usermodel.RolePhysician
	RoleReceptionist = usermodel.RoleReceptionist
	RolePatient      = usermodel.RolePatient
)

var (
	ErrUserNotFound        = usermodel.ErrUserNotFound
	ErrDuplicateEmail       = usermodel.ErrDuplicateEmail
	ErrDuplicateSpecialty   = usermodel.ErrDuplicateSpecialty
	ErrSpecialtyScope       = usermodel.ErrSpecialtyScope
	ErrRoleNotFound         = usermodel.ErrRoleNotFound
	ErrRoleImmutable        = usermodel.ErrRoleImmutable
	ErrDuplicateRole        = usermodel.ErrDuplicateRole
	ErrSystemRole           = usermodel.ErrSystemRole
	ErrRoleInUse            = usermodel.ErrRoleInUse
	ErrRequestNotFound      = usermodel.ErrRequestNotFound
	ErrRequestProcessed     = usermodel.ErrRequestProcessed
	ErrRequestNotPending    = usermodel.ErrRequestNotPending
	ErrAlreadyOnboarded     = usermodel.ErrAlreadyOnboarded
	ErrInvalidCredentials   = usermodel.ErrInvalidCredentials
	ErrEmailTaken           = usermodel.ErrEmailTaken
	ErrEmailInUse           = usermodel.ErrEmailInUse
	ErrCannotDemoteSelf     = usermodel.ErrCannotDemoteSelf
	ErrLastActiveAdmin      = usermodel.ErrLastActiveAdmin
)
