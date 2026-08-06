package auth

import "go.uber.org/fx"

// Module provides session management and CSRF protection.
var Module = fx.Module("auth",
	fx.Provide(NewSessionManager),
	fx.Provide(NewCSRF),
)
