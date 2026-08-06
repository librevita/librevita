package policy

import (
	"context"
	"log/slog"

	"go.uber.org/fx"
)

// Module provides the CEL policy engine used for authorization. Policies
// are seeded and compiled after the database migrations run.
var Module = fx.Module("policy",
	fx.Provide(NewPolicyEngine),
	fx.Invoke(registerLifecycle),
)

func registerLifecycle(lc fx.Lifecycle, pe *PolicyEngine, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return pe.Load(ctx)
		},
	})
}
