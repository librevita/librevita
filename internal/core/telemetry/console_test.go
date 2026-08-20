package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestConsoleHandlerFormatsColumns(t *testing.T) {
	var output bytes.Buffer
	handler := &consoleHandler{
		writer: &consoleWriter{dst: &output, width: 200},
		level:  slog.LevelDebug,
	}
	slog.New(handler).Info("using SQLite/WAL persistence", slog.String("path", "./librevita.db"))

	got := output.String()
	assert.Contains(t, got, `INFO`)
	assert.Contains(t, got, `using SQLite/WAL persistence path=./librevita.db`)
	assert.NotContains(t, got, `"using SQLite/WAL persistence"`)
	assert.NotContains(t, got, "msg=")
	assert.NotContains(t, got, "{")
}

func TestConsoleHandlerWritesUTC(t *testing.T) {
	var output bytes.Buffer
	handler := &consoleHandler{
		writer: &consoleWriter{dst: &output, width: 200},
		level:  slog.LevelDebug,
	}
	loc := time.FixedZone("BRT", -3*60*60)
	_ = handler.Handle(context.Background(), slog.NewRecord(
		time.Date(2026, 8, 6, 15, 4, 5, 0, loc), slog.LevelInfo, "test message", 0,
	))

	got := output.String()
	want := "2026-08-06T18:04:05.000+00:00"
	assert.True(t, strings.HasPrefix(got, want), "output = %q, want UTC prefix %q", got, want)
	assert.False(t, strings.HasPrefix(got, "2026-08-06T15:04:05"), "output wrote local time instead of UTC")
}
