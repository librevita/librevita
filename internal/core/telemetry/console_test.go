package telemetry

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestConsoleWriterTruncatesEachLine(t *testing.T) {
	var output bytes.Buffer
	writer := &consoleWriter{dst: &output, width: 10}
	input := []byte("123456789012345\nshort\n")

	n, err := writer.Write(input)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(input) {
		t.Fatalf("Write returned %d, want %d", n, len(input))
	}
	if got, want := output.String(), "1234567890\nshort\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConsoleHandlerFormatsColumns(t *testing.T) {
	var output bytes.Buffer
	handler := &consoleHandler{
		writer: &consoleWriter{dst: &output, width: 200},
		level:  slog.LevelDebug,
	}
	slog.New(handler).Info("using SQLite/WAL persistence", slog.String("path", "./librevita.db"))

	got := output.String()
	if !strings.Contains(got, `INFO`) || !strings.Contains(got, `using SQLite/WAL persistence path=./librevita.db`) {
		t.Fatalf("output = %q, expected column format", got)
	}
	if strings.Contains(got, `"using SQLite/WAL persistence"`) || strings.Contains(got, "msg=") || strings.Contains(got, "{") {
		t.Fatalf("output = %q, contains structured text or JSON columns", got)
	}
}
