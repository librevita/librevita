// Package telemetry provides the structured logger used by the application.
package telemetry

import (
	"log/slog"
	"os"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"

	"librevita.org/internal/core/config"
)

// LoggerResult exposes slog to application code and zap for lifecycle flush.
type LoggerResult struct {
	fx.Out

	Logger *slog.Logger
	Zap    *zap.Logger
	Sink   *LogSink
}

// NewLogger is the Fx provider for the structured application logger.
// Production uses JSON; development uses bounded text columns.

func NewLogger(cfg *config.Config) (LoggerResult, error) {
	var (
		zapLogger *zap.Logger
		handler   slog.Handler
		sink      *LogSink
		err       error
	)

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
			zapConfig.Level,
		)
		zapLogger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
		handler = zapslog.NewHandler(zapLogger.Core(), zapslog.WithCaller(true))
	} else {
		zapLogger, err = zap.NewDevelopment()
		if err != nil {
			return LoggerResult{}, err
		}
		// Zap's console encoder renders structured context as JSON. Use a
		// column-oriented slog handler for development output instead.
		handler = newConsoleHandler(os.Stderr)
		sink = &LogSink{}
	}

	return LoggerResult{
		Logger: slog.New(handler),
		Zap:    zapLogger,
		Sink:   sink,
	}, nil
}
