package policy

import (
	"context"

	"go.uber.org/fx"

	"librevita.org/internal/domain/clinic/usecase"
)

// Module provides the CEL policy engine used for authorization. Policies
// are seeded and compiled after the database migrations run.
var Module = fx.Module("policy",
	fx.Provide(
		NewPolicyRepository,
		NewPolicyEngine,
	),
	fx.Invoke(registerLifecycle),
	fx.Invoke(wireClinicContext),
)

// wireClinicContext exposes the installation's clinic id as
// context.clinic_id in every policy evaluation (see
// PolicyEngine.SetClockProvider).
func wireClinicContext(pe *PolicyEngine, clocks *usecase.ClockProvider) {
	pe.SetClockProvider(clocks)
}

func registerLifecycle(lc fx.Lifecycle, pe *PolicyEngine) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return pe.Load(ctx)
		},
	})
}
