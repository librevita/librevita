package telemetry

import (
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"librevita.org/internal/core/config"
)

// LogSink owns resources opened for a production log destination.
type LogSink struct {
	closer io.Closer
}

func (s *LogSink) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

func newProductionSink(cfg config.LoggingConfig) (zapcore.WriteSyncer, *LogSink, error) {
	switch cfg.Mode {
	case config.LogModeConsole:
		return zapcore.AddSync(os.Stderr), &LogSink{}, nil

	case config.LogModeFile:
		if err := ensureLogDir(cfg.File.Path); err != nil {
			return nil, nil, err
		}
		file, err := os.OpenFile(cfg.File.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, err
		}
		return zapcore.AddSync(file), &LogSink{closer: file}, nil

	case config.LogModeRotating:
		if err := ensureLogDir(cfg.Rotating.Path); err != nil {
			return nil, nil, err
		}
		rotating := &lumberjack.Logger{
			Filename:   cfg.Rotating.Path,
			MaxSize:    cfg.Rotating.MaxSizeMB,
			MaxBackups: cfg.Rotating.MaxBackups,
			MaxAge:     cfg.Rotating.MaxAgeDays,
			Compress:   cfg.Rotating.Compress,
		}
		return zapcore.AddSync(rotating), &LogSink{closer: rotating}, nil

	default:
		return nil, nil, os.ErrInvalid
	}
}

func ensureLogDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}
