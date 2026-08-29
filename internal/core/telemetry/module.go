package telemetry

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Module provides the logger and flushes it during shutdown.
var Module = fx.Module("telemetry",
	fx.Provide(NewLogger),
	fx.Invoke(func(lc fx.Lifecycle, zapLogger *zap.Logger, sink *LogSink) {
		lc.Append(fx.Hook{
			OnStop: func(context.Context) error {
				// Sync may return EINVAL for stdout/stderr on Linux.
				_ = zapLogger.Sync()
				return sink.Close()
			},
		})
	}),
)

// FxLogger adapts the process Zap logger for Fx event logs.
func FxLogger(zapLogger *zap.Logger) fxevent.Logger {
	l := &fxevent.ZapLogger{Logger: zapLogger}
	l.UseLogLevel(zapcore.DebugLevel)
	l.UseErrorLevel(zapcore.ErrorLevel)
	return l
}
