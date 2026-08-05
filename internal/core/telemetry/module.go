package telemetry

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module provides the logger and flushes it during shutdown.
var Module = fx.Module("telemetry",
	fx.Provide(NewLogger),
	fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger, sink *LogSink) {
		lc.Append(fx.Hook{
			OnStop: func(context.Context) error {
				// Sync may return EINVAL for stdout/stderr on Linux.
				_ = log.Sync()
				return sink.Close()
			},
		})
	}),
)
