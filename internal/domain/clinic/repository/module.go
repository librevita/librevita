package repository

import "go.uber.org/fx"

// Module provides the clinic repository adapter.
var Module = fx.Module("clinic_repository",
	fx.Provide(NewClinicRepository),
)
