package clinic

import (
	"go.uber.org/fx"

	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/clinic/usecase"
)

// Module provides clinic-domain services.
var Module = fx.Module("clinic",
	fx.Provide(repository.NewClinicRepository),
	fx.Provide(usecase.NewClockProvider),
)
