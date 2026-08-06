package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	defaultConsoleWidth = 120
	consoleSourceWidth  = 20
)

type consoleHandler struct {
	writer io.Writer
	level  slog.Level
	groups []string
	attrs  []consoleAttr
}

type consoleAttr struct {
	groups []string
	attr   slog.Attr
}

func newConsoleHandler(dst io.Writer) slog.Handler {
	return &consoleHandler{
		writer: newConsoleWriter(dst),
		level:  slog.LevelDebug,
	}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *consoleHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	line.WriteString(record.Time.UTC().Format("2006-01-02T15:04:05.000-07:00"))
	line.WriteString("   ")
	line.WriteString(fmt.Sprintf("%-5s", strings.ToUpper(record.Level.String())))
	line.WriteByte(' ')

	sourceInfo := record.Source()
	source := "-"
	if sourceInfo != nil && sourceInfo.File != "" {
		source = filepath.Base(sourceInfo.File) + ":" + strconv.Itoa(sourceInfo.Line)
	}
	source = string(truncateRunes([]byte(source), consoleSourceWidth))
	line.WriteString(fmt.Sprintf("%-*s", consoleSourceWidth, source))
	line.WriteString("   ")
	line.WriteString(strings.ReplaceAll(record.Message, "\n", " "))

	for _, attr := range h.attrs {
		appendConsoleAttr(&line, attr.groups, attr.attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendConsoleAttr(&line, h.groups, attr)
		return true
	})
	line.WriteByte('\n')

	_, err := h.writer.Write([]byte(line.String()))
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	for _, attr := range attrs {
		clone.attrs = append(clone.attrs, consoleAttr{
			groups: append([]string(nil), h.groups...),
			attr:   attr,
		})
	}
	return clone
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *consoleHandler) clone() *consoleHandler {
	clone := *h
	clone.groups = append([]string(nil), h.groups...)
	clone.attrs = append([]consoleAttr(nil), h.attrs...)
	return &clone
}

func appendConsoleAttr(line *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		nestedGroups := append(append([]string(nil), groups...), attr.Key)
		for _, child := range attr.Value.Group() {
			appendConsoleAttr(line, nestedGroups, child)
		}
		return
	}

	keyParts := append(append([]string(nil), groups...), attr.Key)
	key := strings.Trim(strings.Join(keyParts, "."), ".")
	if key == "" {
		return
	}

	line.WriteByte(' ')
	line.WriteString(key)
	line.WriteByte('=')
	line.WriteString(formatConsoleValue(attr.Value))
}

func formatConsoleValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return formatConsoleString(value.String())
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindAny:
		if value.Any() == nil {
			return "nil"
		}
		return formatConsoleString(fmt.Sprint(value.Any()))
	default:
		return formatConsoleString(value.String())
	}
}

func formatConsoleString(value string) string {
	if value == "" {
		return strconv.Quote(value)
	}
	for _, char := range value {
		if unicode.IsSpace(char) || strings.ContainsRune("=\"", char) {
			return strconv.Quote(value)
		}
	}
	return value
}

// consoleWriter keeps development logs on one bounded line.
type consoleWriter struct {
	dst   io.Writer
	width int
	mu    sync.Mutex
}

func newConsoleWriter(dst io.Writer) *consoleWriter {
	return &consoleWriter{dst: dst, width: consoleWidth()}
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	inputLen := len(p)

	if w.width <= 0 {
		return w.dst.Write(p)
	}

	var out bytes.Buffer
	for len(p) > 0 {
		lineEnd := bytes.IndexByte(p, '\n')
		line := p
		newline := false
		if lineEnd >= 0 {
			line = p[:lineEnd]
			newline = true
		}

		out.Write(truncateRunes(line, w.width))
		if newline {
			out.WriteByte('\n')
			p = p[lineEnd+1:]
		} else {
			p = nil
		}
	}

	_, err := w.dst.Write(out.Bytes())
	if err != nil {
		return 0, err
	}
	return inputLen, nil
}

func consoleWidth() int {
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}

	if columns, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && columns > 0 {
		return columns
	}
	return defaultConsoleWidth
}

func truncateRunes(line []byte, width int) []byte {
	if width <= 0 || utf8.RuneCount(line) <= width {
		return line
	}
	return []byte(string([]rune(string(line))[:width]))
}
