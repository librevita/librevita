// Package log is the application logging facade. Call sites use typed
// Fields (the same model as Zap) and never import go.uber.org/zap.
package log

import "context"

// Level is a logging severity. Debug is the lowest.
type Level int8

const (
	Debug Level = iota
	Info
	Warn
	// ErrorLevel is the error severity. It is not named Error because
	// Error is the Field constructor, matching Zap.
	ErrorLevel
)

// Logger is the structured logger used across LibreVita.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	DebugContext(ctx context.Context, msg string, fields ...Field)
	InfoContext(ctx context.Context, msg string, fields ...Field)
	WarnContext(ctx context.Context, msg string, fields ...Field)
	ErrorContext(ctx context.Context, msg string, fields ...Field)
	With(fields ...Field) Logger
	Enabled(Level) bool
}

// Nop returns a logger that discards every record.
func Nop() Logger { return nopLogger{} }

type nopLogger struct{}

func (nopLogger) Debug(string, ...Field)                         {}
func (nopLogger) Info(string, ...Field)                          {}
func (nopLogger) Warn(string, ...Field)                          {}
func (nopLogger) Error(string, ...Field)                         {}
func (nopLogger) DebugContext(context.Context, string, ...Field) {}
func (nopLogger) InfoContext(context.Context, string, ...Field)  {}
func (nopLogger) WarnContext(context.Context, string, ...Field)  {}
func (nopLogger) ErrorContext(context.Context, string, ...Field) {}
func (nopLogger) With(...Field) Logger                           { return nopLogger{} }
func (nopLogger) Enabled(Level) bool                             { return false }
