package telemetry

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap/zapcore"
)

const (
	defaultConsoleWidth = 120
	consoleSourceWidth  = 20
)

type consoleCore struct {
	zapcore.LevelEnabler
	writer io.Writer
	fields []zapcore.Field
}

func newConsoleCore(dst io.Writer, level zapcore.LevelEnabler) zapcore.Core {
	return &consoleCore{
		LevelEnabler: level,
		writer:       newConsoleWriter(dst),
	}
}

func (c *consoleCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	clone.fields = append(append([]zapcore.Field(nil), c.fields...), fields...)
	return &clone
}

func (c *consoleCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *consoleCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	var line strings.Builder
	line.WriteString(ent.Time.UTC().Format("2006-01-02T15:04:05.000-07:00"))
	line.WriteString("   ")
	fmt.Fprintf(&line, "%-5s", strings.ToUpper(ent.Level.String()))
	line.WriteByte(' ')

	source := "-"
	if ent.Caller.Defined {
		source = filepath.Base(ent.Caller.File) + ":" + strconv.Itoa(ent.Caller.Line)
	}
	source = string(truncateRunes([]byte(source), consoleSourceWidth))
	fmt.Fprintf(&line, "%-*s", consoleSourceWidth, source)
	line.WriteString("   ")
	line.WriteString(strings.ReplaceAll(ent.Message, "\n", " "))

	for _, field := range c.fields {
		appendConsoleField(&line, field)
	}
	for _, field := range fields {
		appendConsoleField(&line, field)
	}
	line.WriteByte('\n')

	_, err := c.writer.Write([]byte(line.String()))
	return err
}

func (c *consoleCore) Sync() error {
	if syncer, ok := c.writer.(zapcore.WriteSyncer); ok {
		return syncer.Sync()
	}
	return nil
}

func appendConsoleField(line *strings.Builder, field zapcore.Field) {
	if field.Type == zapcore.SkipType || field.Type == zapcore.NamespaceType {
		return
	}
	line.WriteByte(' ')
	line.WriteString(field.Key)
	line.WriteByte('=')
	line.WriteString(formatConsoleField(field))
}

func formatConsoleField(field zapcore.Field) string {
	switch field.Type {
	case zapcore.StringType, zapcore.ByteStringType:
		return formatConsoleString(field.String)
	case zapcore.BoolType:
		return strconv.FormatBool(field.Integer == 1)
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type,
		zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type,
		zapcore.UintptrType:
		return strconv.FormatInt(field.Integer, 10)
	case zapcore.Float64Type:
		return strconv.FormatFloat(math.Float64frombits(uint64(field.Integer)), 'g', -1, 64) // #nosec G115 -- IEEE-754 bit pattern stored in int64
	case zapcore.Float32Type:
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(field.Integer))), 'g', -1, 32) // #nosec G115 -- IEEE-754 bit pattern stored in int64
	case zapcore.DurationType:
		return time.Duration(field.Integer).String()
	case zapcore.TimeType:
		t := time.Unix(0, field.Integer)
		if loc, ok := field.Interface.(*time.Location); ok && loc != nil {
			t = t.In(loc)
		}
		return t.UTC().Format(time.RFC3339Nano)
	case zapcore.ErrorType:
		if field.Interface == nil {
			return "nil"
		}
		if err, ok := field.Interface.(error); ok {
			return formatConsoleString(err.Error())
		}
		return formatConsoleString(fmt.Sprint(field.Interface))
	case zapcore.StringerType:
		if s, ok := field.Interface.(fmt.Stringer); ok && s != nil {
			return formatConsoleString(s.String())
		}
		return "nil"
	default:
		if field.Interface != nil {
			return formatConsoleString(fmt.Sprint(field.Interface))
		}
		if field.String != "" {
			return formatConsoleString(field.String)
		}
		return strconv.FormatInt(field.Integer, 10)
	}
}

func formatConsoleString(value string) string {
	if value == "" {
		return strconv.Quote(value)
	}
	for _, char := range value {
		if unicode.IsSpace(char) || char == '=' || char == '"' {
			return strconv.Quote(value)
		}
	}
	return value
}

func consoleWidth() int {
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	return defaultConsoleWidthForFd()
}
