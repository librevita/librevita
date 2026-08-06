package policy

import "go.uber.org/fx"

// Module provides the CEL policy engine used for authorization.
var Module = fx.Module("policy",
	fx.Provide(NewPolicyEngine),
)
