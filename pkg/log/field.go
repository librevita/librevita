package log

import (
	"time"

	"go.uber.org/zap"
)

// Field is a typed structured log attribute.
type Field = zap.Field

// String constructs a string Field.
func String(key, value string) Field { return zap.String(key, value) }

// Strings constructs a string-slice Field.
func Strings(key string, value []string) Field { return zap.Strings(key, value) }

// Int constructs an int Field.
func Int(key string, value int) Field { return zap.Int(key, value) }

// Int64 constructs an int64 Field.
func Int64(key string, value int64) Field { return zap.Int64(key, value) }

// Bool constructs a bool Field.
func Bool(key string, value bool) Field { return zap.Bool(key, value) }

// Duration constructs a time.Duration Field.
func Duration(key string, value time.Duration) Field { return zap.Duration(key, value) }

// Time constructs a time.Time Field.
func Time(key string, value time.Time) Field { return zap.Time(key, value) }

// Any constructs a Field from an arbitrary value.
func Any(key string, value any) Field { return zap.Any(key, value) }

// Stringer constructs a Field from a fmt.Stringer.
func Stringer(key string, value interface{ String() string }) Field {
	return zap.Stringer(key, value)
}

func NamedError(key string, err error) Field { return zap.NamedError(key, err) }

// Error constructs a Field for err under the key "error".
func Error(err error) Field { return zap.Error(err) }
