package log

import "context"

type contextKey struct{}

// WithRequestID stores the HTTP request id on ctx so *Context log
// methods can attach it as a Field.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, id)
}

// RequestID returns the request id stored on ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

// FieldsWithContext prepends the request id Field when ctx carries one.
func FieldsWithContext(ctx context.Context, fields []Field) []Field {
	if ctx == nil {
		return fields
	}
	id := RequestID(ctx)
	if id == "" {
		return fields
	}
	out := make([]Field, 0, len(fields)+1)
	out = append(out, String("request_id", id))
	return append(out, fields...)
}
