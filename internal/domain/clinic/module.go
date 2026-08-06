package clinic

import "go.uber.org/fx"

// Module provides clinic-domain services.
var Module = fx.Module("clinic",
	fx.Provide(NewClockProvider),
)
