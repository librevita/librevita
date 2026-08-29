package log

import (
	"context"
	"sync"
)

// Record is one captured log line, used in tests.
type Record struct {
	Level   Level
	Message string
	Fields  []Field
}

// Recorder is a Logger that stores records in memory.
type Recorder struct {
	mu      sync.Mutex
	level   Level
	records []Record
}

// NewRecorder returns a Logger that records every enabled line.
func NewRecorder() *Recorder {
	return &Recorder{level: Debug}
}

// Records returns a copy of the captured lines.
func (r *Recorder) Records() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, len(r.records))
	copy(out, r.records)
	return out
}

// Messages returns the messages of captured records.
func (r *Recorder) Messages() []string {
	recs := r.Records()
	out := make([]string, len(recs))
	for i, rec := range recs {
		out[i] = rec.Message
	}
	return out
}

func (r *Recorder) Debug(msg string, fields ...Field) {
	r.add(Debug, msg, fields)
}
func (r *Recorder) Info(msg string, fields ...Field) { r.add(Info, msg, fields) }
func (r *Recorder) Warn(msg string, fields ...Field) { r.add(Warn, msg, fields) }
func (r *Recorder) Error(msg string, fields ...Field) {
	r.add(ErrorLevel, msg, fields)
}
func (r *Recorder) DebugContext(ctx context.Context, msg string, fields ...Field) {
	r.add(Debug, msg, FieldsWithContext(ctx, fields))
}
func (r *Recorder) InfoContext(ctx context.Context, msg string, fields ...Field) {
	r.add(Info, msg, FieldsWithContext(ctx, fields))
}
func (r *Recorder) WarnContext(ctx context.Context, msg string, fields ...Field) {
	r.add(Warn, msg, FieldsWithContext(ctx, fields))
}
func (r *Recorder) ErrorContext(ctx context.Context, msg string, fields ...Field) {
	r.add(ErrorLevel, msg, FieldsWithContext(ctx, fields))
}

func (r *Recorder) With(fields ...Field) Logger {
	return &prefixedRecorder{parent: r, prefix: append([]Field(nil), fields...)}
}

func (r *Recorder) Enabled(level Level) bool { return level >= r.level }

func (r *Recorder) add(level Level, msg string, fields []Field) {
	if !r.Enabled(level) {
		return
	}
	copied := append([]Field(nil), fields...)
	r.mu.Lock()
	r.records = append(r.records, Record{Level: level, Message: msg, Fields: copied})
	r.mu.Unlock()
}

type prefixedRecorder struct {
	parent *Recorder
	prefix []Field
}

func (p *prefixedRecorder) Debug(msg string, fields ...Field) {
	p.parent.add(Debug, msg, p.merge(fields))
}
func (p *prefixedRecorder) Info(msg string, fields ...Field) {
	p.parent.add(Info, msg, p.merge(fields))
}
func (p *prefixedRecorder) Warn(msg string, fields ...Field) {
	p.parent.add(Warn, msg, p.merge(fields))
}
func (p *prefixedRecorder) Error(msg string, fields ...Field) {
	p.parent.add(ErrorLevel, msg, p.merge(fields))
}
func (p *prefixedRecorder) DebugContext(ctx context.Context, msg string, fields ...Field) {
	p.parent.add(Debug, msg, FieldsWithContext(ctx, p.merge(fields)))
}
func (p *prefixedRecorder) InfoContext(ctx context.Context, msg string, fields ...Field) {
	p.parent.add(Info, msg, FieldsWithContext(ctx, p.merge(fields)))
}
func (p *prefixedRecorder) WarnContext(ctx context.Context, msg string, fields ...Field) {
	p.parent.add(Warn, msg, FieldsWithContext(ctx, p.merge(fields)))
}
func (p *prefixedRecorder) ErrorContext(ctx context.Context, msg string, fields ...Field) {
	p.parent.add(ErrorLevel, msg, FieldsWithContext(ctx, p.merge(fields)))
}
func (p *prefixedRecorder) With(fields ...Field) Logger {
	return &prefixedRecorder{parent: p.parent, prefix: p.merge(fields)}
}
func (p *prefixedRecorder) Enabled(level Level) bool { return p.parent.Enabled(level) }

func (p *prefixedRecorder) merge(fields []Field) []Field {
	out := make([]Field, 0, len(p.prefix)+len(fields))
	out = append(out, p.prefix...)
	return append(out, fields...)
}
