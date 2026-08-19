package repository

import "go.uber.org/fx"

// Module provides the user domain repositories.
var Module = fx.Module("user_repository",
	fx.Provide(
		NewUserRepository,
		NewRoleRepository,
		NewSpecialtyRepository,
		NewStaffRequestRepository,
		NewSetupRepository,
	),
)
