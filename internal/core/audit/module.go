package audit

import "go.uber.org/fx"

// Module provides the audit logger.
var Module = fx.Module("audit",
	fx.Provide(NewLogger),
)
