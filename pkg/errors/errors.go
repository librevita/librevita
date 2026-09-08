package errors

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
)

// Re-export standard library error functions.
var (
	Is     = errors.Is
	As     = errors.As
	Join   = errors.Join
	Unwrap = errors.Unwrap
)

// New creates a new error with a stack trace.
func New(message string) error {
	return withStack(errors.New(message), 1)
}

// Newf formats an error message and attaches a stack trace.
func Newf(format string, args ...any) error {
	return withStack(fmt.Errorf(format, args...), 1)
}

// Wrap wraps an error with a message and attaches a stack trace.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return withStack(fmt.Errorf("%s: %w", message, err), 1)
}

// Wrapf wraps an error with a formatted message and attaches a stack trace.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return withStack(fmt.Errorf("%s: %w", msg, err), 1)
}

// Safe wraps an error, string, or value marked safe (PII-free). In LibreVita it preserves the value.
func Safe[T any](v T) T {
	return v
}

// CombineErrors combines two errors into one using errors.Join.
func CombineErrors(err1, err2 error) error {
	if err1 == nil {
		return err2
	}
	if err2 == nil {
		return err1
	}
	return errors.Join(err1, err2)
}

// Hint support (RFC 7807 problem details)
type hintError struct {
	cause error
	hint  string
}

func (h *hintError) Error() string { return h.cause.Error() }
func (h *hintError) Unwrap() error { return h.cause }
func (h *hintError) Hint() string  { return h.hint }

func (h *hintError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%+v\n(hint: %s)", h.cause, h.hint)
			return
		}
		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(s, h.Error())
	}
}

// WithHint attaches an actionable remediation hint to err.
func WithHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	return &hintError{cause: err, hint: hint}
}

// FlattenHints traverses the error chain and concatenates all hints.
func FlattenHints(err error) string {
	var hints []string
	for curr := err; curr != nil; curr = errors.Unwrap(curr) {
		if h, ok := curr.(interface{ Hint() string }); ok {
			hint := strings.TrimSpace(h.Hint())
			if hint != "" {
				hints = append(hints, hint)
			}
		}
	}
	return strings.Join(hints, " — ")
}

// Secondary error support
type secondaryError struct {
	cause     error
	secondary error
}

func (s *secondaryError) Error() string    { return s.cause.Error() }
func (s *secondaryError) Unwrap() error    { return s.cause }
func (s *secondaryError) Secondary() error { return s.secondary }

// WithSecondaryError attaches a secondary error to err.
func WithSecondaryError(err, secondary error) error {
	if err == nil {
		return nil
	}
	return &secondaryError{cause: err, secondary: secondary}
}

// Stack trace support
type stackError struct {
	cause error
	stack []uintptr
}

func (s *stackError) Error() string { return s.cause.Error() }
func (s *stackError) Unwrap() error { return s.cause }

func (s *stackError) Format(st fmt.State, verb rune) {
	switch verb {
	case 'v':
		if st.Flag('+') {
			_, _ = io.WriteString(st, s.cause.Error())
			frames := runtime.CallersFrames(s.stack)
			for {
				frame, more := frames.Next()
				if !strings.Contains(frame.File, "runtime/") {
					_, _ = fmt.Fprintf(st, "\n\t%s:%d %s", frame.File, frame.Line, frame.Function)
				}
				if !more {
					break
				}
			}
			return
		}
		fallthrough
	case 's', 'q':
		_, _ = io.WriteString(st, s.Error())
	}
}

func withStack(err error, skip int) error {
	var pcs [32]uintptr
	n := runtime.Callers(skip+2, pcs[:])
	return &stackError{cause: err, stack: pcs[:n]}
}
