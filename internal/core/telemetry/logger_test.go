package telemetry_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/telemetry"
	"librevita.org/pkg/log"
)

func TestNewLogger(t *testing.T) {
	// 1. Development mode
	devCfg := &config.Config{
		Mode: "development",
		Logging: config.LoggingConfig{
			Level: "debug",
		},
	}
	resDev, err := telemetry.NewLogger(devCfg)
	require.NoError(t, err)
	require.NotNil(t, resDev.Logger)
	require.NotNil(t, resDev.Zap)
	assert.True(t, resDev.Logger.Enabled(log.Debug))

	ctx := context.Background()
	resDev.Logger.Debug("test debug")
	resDev.Logger.Info("test info")
	resDev.Logger.Warn("test warn")
	resDev.Logger.Error("test error")
	resDev.Logger.DebugContext(ctx, "test debug ctx", log.String("k", "v"))
	resDev.Logger.InfoContext(ctx, "test info ctx", log.String("k", "v"))
	resDev.Logger.WarnContext(ctx, "test warn ctx", log.String("k", "v"))
	resDev.Logger.ErrorContext(ctx, "test error ctx", log.String("k", "v"))

	child := resDev.Logger.With(log.String("component", "test"))
	child.Info("from child logger")

	// 2. Production mode - Console
	prodConsoleCfg := &config.Config{
		Mode: "production",
		Logging: config.LoggingConfig{
			Mode:  config.LogModeConsole,
			Level: "info",
		},
	}
	resProd, err := telemetry.NewLogger(prodConsoleCfg)
	require.NoError(t, err)
	require.NotNil(t, resProd.Logger)

	// 3. Production mode - File
	logFile := filepath.Join(t.TempDir(), "logs", "app.log")
	prodFileCfg := &config.Config{
		Mode: "production",
		Logging: config.LoggingConfig{
			Mode: config.LogModeFile,
			File: config.FileLogConfig{
				Path: logFile,
			},
			Level: "warn",
		},
	}
	resFile, err := telemetry.NewLogger(prodFileCfg)
	require.NoError(t, err)
	resFile.Logger.Warn("file warning")
	require.NoError(t, resFile.Sink.Close())

	// 4. Production mode - Rotating
	rotFile := filepath.Join(t.TempDir(), "rot", "rot.log")
	prodRotCfg := &config.Config{
		Mode: "production",
		Logging: config.LoggingConfig{
			Mode: config.LogModeRotating,
			Rotating: config.RotatingLogConfig{
				Path:       rotFile,
				MaxSizeMB:  10,
				MaxBackups: 3,
				MaxAgeDays: 7,
				Compress:   true,
			},
			Level: "error",
		},
	}
	resRot, err := telemetry.NewLogger(prodRotCfg)
	require.NoError(t, err)
	resRot.Logger.Error("rot error")
	require.NoError(t, resRot.Sink.Close())

	// 5. Invalid mode
	prodInvalidCfg := &config.Config{
		Mode: "production",
		Logging: config.LoggingConfig{
			Mode: "invalid_mode",
		},
	}
	_, err = telemetry.NewLogger(prodInvalidCfg)
	assert.Error(t, err)

	// 6. LogSink close edge cases
	var nilSink *telemetry.LogSink
	assert.NoError(t, nilSink.Close())
	emptySink := &telemetry.LogSink{}
	assert.NoError(t, emptySink.Close())
}

func TestFxLoggerAndModule(t *testing.T) {
	devCfg := &config.Config{
		Mode: "development",
		Logging: config.LoggingConfig{
			Level: "debug",
		},
	}
	resDev, err := telemetry.NewLogger(devCfg)
	require.NoError(t, err)

	fxLog := telemetry.FxLogger(resDev.Zap)
	assert.NotNil(t, fxLog)
	assert.NotNil(t, telemetry.Module)
}
