package config

import "go.uber.org/fx"

// Module provides *Config to the Fx graph.
var Module = fx.Module("config",
	fx.Provide(New),
)
