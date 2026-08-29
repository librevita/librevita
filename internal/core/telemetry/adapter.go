package telemetry

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"librevita.org/pkg/log"
)

type appLogger struct {
	z *zap.Logger
}

func newAppLogger(z *zap.Logger) log.Logger {
	return &appLogger{z: z}
}

func (l *appLogger) Debug(msg string, fields ...log.Field) {
	l.z.Debug(msg, fields...)
}
func (l *appLogger) Info(msg string, fields ...log.Field) {
	l.z.Info(msg, fields...)
}
func (l *appLogger) Warn(msg string, fields ...log.Field) {
	l.z.Warn(msg, fields...)
}
func (l *appLogger) Error(msg string, fields ...log.Field) {
	l.z.Error(msg, fields...)
}

func (l *appLogger) DebugContext(ctx context.Context, msg string, fields ...log.Field) {
	l.z.Debug(msg, log.FieldsWithContext(ctx, fields)...)
}
func (l *appLogger) InfoContext(ctx context.Context, msg string, fields ...log.Field) {
	l.z.Info(msg, log.FieldsWithContext(ctx, fields)...)
}
func (l *appLogger) WarnContext(ctx context.Context, msg string, fields ...log.Field) {
	l.z.Warn(msg, log.FieldsWithContext(ctx, fields)...)
}
func (l *appLogger) ErrorContext(ctx context.Context, msg string, fields ...log.Field) {
	l.z.Error(msg, log.FieldsWithContext(ctx, fields)...)
}

func (l *appLogger) With(fields ...log.Field) log.Logger {
	return &appLogger{z: l.z.With(fields...)}
}

func (l *appLogger) Enabled(level log.Level) bool {
	return l.z.Core().Enabled(zapLevel(level))
}

func zapLevel(level log.Level) zapcore.Level {
	switch level {
	case log.Debug:
		return zapcore.DebugLevel
	case log.Warn:
		return zapcore.WarnLevel
	case log.ErrorLevel:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
