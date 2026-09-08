package errors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorsBasics(t *testing.T) {
	base := New("base error")
	assert.Equal(t, "base error", base.Error())

	wrapped := Wrap(base, "context")
	assert.Equal(t, "context: base error", wrapped.Error())
	assert.True(t, Is(wrapped, base))

	wrappedf := Wrapf(base, "context %d", 42)
	assert.Equal(t, "context 42: base error", wrappedf.Error())
	assert.True(t, Is(wrappedf, base))

	newf := Newf("hello %s", "world")
	assert.Equal(t, "hello world", newf.Error())

	assert.Nil(t, Wrap(nil, "noop"))
	assert.Nil(t, Wrapf(nil, "noop %d", 1))
}

func TestErrorsHints(t *testing.T) {
	base := New("db failed")
	withH1 := WithHint(base, "Check connection string")
	withH2 := WithHint(withH1, "Verify credentials")

	hints := FlattenHints(withH2)
	assert.Contains(t, hints, "Verify credentials")
	assert.Contains(t, hints, "Check connection string")

	assert.Nil(t, WithHint(nil, "noop"))
	assert.Equal(t, "", FlattenHints(base))
}

func TestErrorsSecondary(t *testing.T) {
	pri := New("primary")
	sec := New("secondary")
	err := WithSecondaryError(pri, sec)

	assert.Equal(t, "primary", err.Error())
	assert.True(t, Is(err, pri))

	secErr, ok := err.(interface{ Secondary() error })
	require.True(t, ok)
	assert.Equal(t, sec, secErr.Secondary())

	assert.Nil(t, WithSecondaryError(nil, sec))
}

func TestErrorsFormattingAndStack(t *testing.T) {
	err := New("stack test")
	formatted := fmt.Sprintf("%+v", err)
	assert.True(t, strings.Contains(formatted, "stack test"))
	assert.True(t, strings.Contains(formatted, "errors_test.go"))
}

func TestErrorsSafeAndCombine(t *testing.T) {
	err := New("sensitive")
	assert.Equal(t, err, Safe(err))

	e1 := New("first")
	e2 := New("second")
	combined := CombineErrors(e1, e2)
	assert.True(t, Is(combined, e1))
	assert.True(t, Is(combined, e2))
}
