package telemetry

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"librevita.org/pkg/log"
)

func TestConsoleWriterTruncatesEachLine(t *testing.T) {
	var output bytes.Buffer
	writer := &consoleWriter{dst: &output, width: 10}
	input := []byte("123456789012345\nshort\n")

	n, err := writer.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, "1234567890\nshort\n", output.String())
}

func TestConsoleCoreFormatsColumns(t *testing.T) {
	var output bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.DebugLevel,
		writer:       &consoleWriter{dst: &output, width: 200},
	}
	logger := newAppLogger(mustZap(core))
	logger.Info("using SQLite/WAL persistence", log.String("path", "./librevita.db"))

	got := output.String()
	assert.Contains(t, got, `INFO`)
	assert.Contains(t, got, `using SQLite/WAL persistence path=./librevita.db`)
	assert.NotContains(t, got, `"using SQLite/WAL persistence"`)
	assert.NotContains(t, got, "msg=")
	assert.NotContains(t, got, "{")
}

func TestConsoleCoreWritesUTC(t *testing.T) {
	var output bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.DebugLevel,
		writer:       &consoleWriter{dst: &output, width: 200},
	}
	loc := time.FixedZone("BRT", -3*60*60)
	err := core.Write(zapcore.Entry{
		Time:    time.Date(2026, 8, 6, 15, 4, 5, 0, loc),
		Level:   zapcore.InfoLevel,
		Message: "test message",
	}, nil)
	require.NoError(t, err)

	got := output.String()
	want := "2026-08-06T18:04:05.000+00:00"
	assert.True(t, strings.HasPrefix(got, want), "output = %q, want UTC prefix %q", got, want)
	assert.False(t, strings.HasPrefix(got, "2026-08-06T15:04:05"), "output wrote local time instead of UTC")
}

func TestConsoleCoreRespectsLevel(t *testing.T) {
	var output bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.InfoLevel,
		writer:       &consoleWriter{dst: &output, width: 200},
	}
	logger := newAppLogger(mustZap(core))
	assert.False(t, logger.Enabled(log.Debug))
	logger.Debug("hidden")
	logger.Info("shown")
	assert.NotContains(t, output.String(), "hidden")
	assert.Contains(t, output.String(), "shown")
}

func TestAppLoggerAttachesRequestID(t *testing.T) {
	var output bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.DebugLevel,
		writer:       &consoleWriter{dst: &output, width: 400},
	}
	logger := newAppLogger(mustZap(core))
	ctx := log.WithRequestID(t.Context(), "rid-9")
	logger.ErrorContext(ctx, "failed", log.String("system", "cpf"))
	got := output.String()
	assert.Contains(t, got, "request_id=rid-9")
	assert.Contains(t, got, "system=cpf")
}

func mustZap(core zapcore.Core) *zap.Logger {
	return zap.New(core)
}
