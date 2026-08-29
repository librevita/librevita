package log

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNopDiscards(t *testing.T) {
	l := Nop()
	l.Info("ignored", String("k", "v"))
	l.ErrorContext(context.Background(), "ignored")
	assert.False(t, l.Enabled(Debug))
	assert.False(t, l.Enabled(ErrorLevel))
}

func TestRecorderCapturesFieldsAndContext(t *testing.T) {
	rec := NewRecorder()
	ctx := WithRequestID(context.Background(), "req-1")
	rec.ErrorContext(ctx, "failed", String("system", "cpf"))

	got := rec.Records()
	require.Len(t, got, 1)
	assert.Equal(t, ErrorLevel, got[0].Level)
	assert.Equal(t, "failed", got[0].Message)
	require.Len(t, got[0].Fields, 2)
	assert.Equal(t, "request_id", got[0].Fields[0].Key)
	assert.Equal(t, "req-1", got[0].Fields[0].String)
	assert.Equal(t, "system", got[0].Fields[1].Key)
	assert.Equal(t, "cpf", got[0].Fields[1].String)
}

func TestRecorderWithPrefixesFields(t *testing.T) {
	rec := NewRecorder()
	rec.With(String("clinic_id", "c1")).Info("loaded")

	got := rec.Records()
	require.Len(t, got, 1)
	require.Len(t, got[0].Fields, 1)
	assert.Equal(t, "clinic_id", got[0].Fields[0].Key)
}

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, RequestID(ctx))
	assert.Empty(t, RequestID(WithRequestID(ctx, "")))
	assert.Equal(t, "abc", RequestID(WithRequestID(ctx, "abc")))
}

func TestLoggerMethodsTakeFields(t *testing.T) {
	iface := reflect.TypeOf((*Logger)(nil)).Elem()
	fieldSlice := reflect.TypeOf([]Field(nil))
	for _, name := range []string{
		"Debug", "Info", "Warn", "Error",
		"DebugContext", "InfoContext", "WarnContext", "ErrorContext",
		"With",
	} {
		m, ok := iface.MethodByName(name)
		require.True(t, ok, name)
		require.True(t, m.Type.IsVariadic(), name)
		last := m.Type.In(m.Type.NumIn() - 1)
		assert.Equal(t, fieldSlice, last, name)
	}
}
