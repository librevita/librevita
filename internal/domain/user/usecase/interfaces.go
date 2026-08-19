package usecase

import (
	usermodel "librevita.org/internal/domain/user/model"
)

// Re-export repository interfaces from user/model.
type (
	UserRepository         = usermodel.UserRepository
	RoleRepository         = usermodel.RoleRepository
	SpecialtyRepository     = usermodel.SpecialtyRepository
	StaffRequestRepository = usermodel.StaffRequestRepository
	SetupRepository        = usermodel.SetupRepository
)
