// Package audit provides a durable, tamper-evident-by-design audit trail
// for authentication, authorization, and future clinical operations.
//
// Recording is best-effort: a failed INSERT is logged through slog and does
// not fail the operation that triggered it. This package is
// transport-agnostic.
package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Outcome of an audited operation.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// Event is one audit record. Sensitive values (passwords, tokens, CSRF)
// must never be placed in Detail.
type Event struct {
	ActorID   int64  // 0 when anonymous.
	ActorMail string // Best-effort actor identity.
	Action    string // e.g. "register", "login", "logout", "authorize".
	Resource  string // e.g. "user", "session", "policy:admin.view".
	Result    string // ResultSuccess or ResultFailure.
	IP        string
	RequestID string
	Detail    string
}

// Logger persists audit events to SQLite.
type Logger struct {
	db  *sql.DB
	log *slog.Logger
}

// NewLogger is the Fx provider. It requires the SQLite backend, like the
// session store.
func NewLogger(db *sql.DB, log *slog.Logger) (*Logger, error) {
	if db == nil {
		return nil, fmt.Errorf("audit: requires the SQLite backend")
	}
	return &Logger{db: db, log: log}, nil
}

// Record persists ev. Failures are logged and swallowed so that auditing
// never breaks the audited operation.
func (l *Logger) Record(ctx context.Context, ev Event) {
	var actorID any
	if ev.ActorID != 0 {
		actorID = ev.ActorID
	}

	_, err := l.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_id, actor_email, action, resource, result, ip, request_id, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		actorID, ev.ActorMail, ev.Action, ev.Resource, ev.Result, ev.IP, ev.RequestID, ev.Detail,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	)
	if err != nil {
		l.log.Error("audit record failed",
			"action", ev.Action, "resource", ev.Resource, "error", err)
	}
}
