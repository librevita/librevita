// Package telemetry provides the structured logger used by the application.
package telemetry

import (
	"os"
	"strings"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"librevita.org/internal/core/config"
	"librevita.org/pkg/log"
)

// LoggerResult exposes the application logger and zap for lifecycle flush.
type LoggerResult struct {
	fx.Out

	Logger log.Logger
	Zap    *zap.Logger
	Sink   *LogSink
}

// NewLogger is the Fx provider for the structured application logger.
// Production uses JSON; development uses bounded text columns.
func NewLogger(cfg *config.Config) (LoggerResult, error) {
	var (
		zapLogger *zap.Logger
		sink      *LogSink
		err       error
	)

	level := zapLevelFromConfig(cfg.Logging.Level)

	if cfg.IsProduction() {
		zapConfig := zap.NewProductionConfig()
		zapConfig.EncoderConfig.CallerKey = "code"
		var output zapcore.WriteSyncer
		output, sink, err = newProductionSink(cfg.Logging)
		if err != nil {
			return LoggerResult{}, err
		}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zapConfig.EncoderConfig),
			output,
			level,
		)
		zapLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	} else {
		zapLogger = zap.New(newConsoleCore(os.Stderr, level), zap.AddCaller(), zap.AddCallerSkip(1))
		sink = &LogSink{}
	}

	return LoggerResult{
		Logger: newAppLogger(zapLogger),
		Zap:    zapLogger,
		Sink:   sink,
	}, nil
}

func zapLevelFromConfig(level string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case config.LogLevelDebug:
		return zapcore.DebugLevel
	case config.LogLevelWarn:
		return zapcore.WarnLevel
	case config.LogLevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
