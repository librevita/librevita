package telemetry

import (
	"bytes"
	"errors"
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

func TestFormatConsoleFieldAllTypes(t *testing.T) {
	tests := []struct {
		name   string
		field  zapcore.Field
		expect string
	}{
		{"string", zap.String("k", "simple"), "simple"},
		{"string_with_space", zap.String("k", "hello world"), `"hello world"`},
		{"string_empty", zap.String("k", ""), `""`},
		{"bool_true", zap.Bool("k", true), "true"},
		{"bool_false", zap.Bool("k", false), "false"},
		{"int64", zap.Int64("k", 42), "42"},
		{"int32", zap.Int32("k", -7), "-7"},
		{"int16", zap.Int16("k", 100), "100"},
		{"int8", zap.Int8("k", 3), "3"},
		{"uint64", zap.Uint64("k", 99), "99"},
		{"uint32", zap.Uint32("k", 1), "1"},
		{"uint16", zap.Uint16("k", 0), "0"},
		{"uint8", zap.Uint8("k", 255), "255"},
		{"float64", zap.Float64("k", 3.14), "3.14"},
		{"float32", zap.Float32("k", 2.5), "2.5"},
		{"duration", zap.Duration("k", 5*time.Second), "5s"},
		{"time", zap.Time("k", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)), "2025-01-01T12:00:00Z"},
		{"error_nil_interface", zapcore.Field{Key: "error", Type: zapcore.ErrorType, Interface: nil}, "nil"},
		{"error_real", zapcore.Field{Key: "error", Type: zapcore.ErrorType, Interface: errors.New("oops")}, "oops"},
		{"error_with_space", zapcore.Field{Key: "error", Type: zapcore.ErrorType, Interface: errors.New("big fail here")}, `"big fail here"`},
		{"stringer", zap.Stringer("k", stringerVal("hello")), "hello"},
		{"stringer_nil", zap.Stringer("k", nil), "nil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatConsoleField(tt.field)
			assert.Equal(t, tt.expect, got)
		})
	}
}

type stringerVal string

func (s stringerVal) String() string { return string(s) }

func TestAppendConsoleFieldSkipsTypes(t *testing.T) {
	var b strings.Builder
	appendConsoleField(&b, zapcore.Field{Type: zapcore.SkipType, Key: "skip"})
	assert.Empty(t, b.String())

	appendConsoleField(&b, zapcore.Field{Type: zapcore.NamespaceType, Key: "ns"})
	assert.Empty(t, b.String())

	appendConsoleField(&b, zap.String("ok", "val"))
	assert.Contains(t, b.String(), "ok=val")
}

func TestFormatConsoleStringQuoting(t *testing.T) {
	assert.Equal(t, `""`, formatConsoleString(""))
	assert.Equal(t, "simple", formatConsoleString("simple"))
	assert.Equal(t, `"has space"`, formatConsoleString("has space"))
	assert.Equal(t, `"has=eq"`, formatConsoleString("has=eq"))
	assert.Equal(t, `"has\"quote"`, formatConsoleString(`has"quote`))
}

func TestConsoleWriterZeroWidth(t *testing.T) {
	var buf bytes.Buffer
	w := &consoleWriter{dst: &buf, width: 0}
	input := []byte("this line should not be truncated\n")
	n, err := w.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, string(input), buf.String())
}

func TestConsoleWriterNoTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	w := &consoleWriter{dst: &buf, width: 5}
	n, err := w.Write([]byte("1234567"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.Equal(t, "12345", buf.String())
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, []byte("abc"), truncateRunes([]byte("abcdef"), 3))
	assert.Equal(t, []byte("ab"), truncateRunes([]byte("ab"), 5))
	assert.Equal(t, []byte("ab"), truncateRunes([]byte("ab"), 0))
}

func TestConsoleWidthFromEnv(t *testing.T) {
	t.Setenv("COLUMNS", "200")
	w := consoleWidth()
	assert.Equal(t, 200, w)
}

func TestConsoleCoreWithFieldsAndSync(t *testing.T) {
	var buf bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.DebugLevel,
		writer:       &consoleWriter{dst: &buf, width: 300},
	}

	// With creates a new core with added fields
	child := core.With([]zapcore.Field{zap.String("comp", "test")})
	assert.NotNil(t, child)

	// Write through child core should include the With fields
	err := child.Write(zapcore.Entry{
		Time:    time.Now(),
		Level:   zapcore.InfoLevel,
		Message: "with-fields",
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "comp=test")
	assert.Contains(t, buf.String(), "with-fields")

	// Sync on non-syncer returns nil
	assert.NoError(t, core.Sync())
}

func TestConsoleCoreCaller(t *testing.T) {
	var buf bytes.Buffer
	core := &consoleCore{
		LevelEnabler: zapcore.DebugLevel,
		writer:       &consoleWriter{dst: &buf, width: 300},
	}
	err := core.Write(zapcore.Entry{
		Time:    time.Now(),
		Level:   zapcore.WarnLevel,
		Message: "with caller",
		Caller:  zapcore.EntryCaller{Defined: true, File: "/some/path/to/file.go", Line: 42},
	}, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "file.go:42")
	assert.Contains(t, buf.String(), "WARN")
}

func TestZapLevelMapping(t *testing.T) {
	assert.Equal(t, zapcore.DebugLevel, zapLevel(log.Debug))
	assert.Equal(t, zapcore.WarnLevel, zapLevel(log.Warn))
	assert.Equal(t, zapcore.ErrorLevel, zapLevel(log.ErrorLevel))
	assert.Equal(t, zapcore.InfoLevel, zapLevel(log.Level(99)))
}

