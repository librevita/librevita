package audit

import "go.uber.org/fx"

// Module provides the audit logging service and repository.
var Module = fx.Module("audit",
	fx.Provide(
		NewAuditRepository,
		NewLogger,
	),
)
