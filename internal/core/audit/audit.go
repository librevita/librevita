// Package audit implements the tamper-evident append-only audit trail.
package audit

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"

	"librevita.org/internal/core/clinicctx"
)

// Event is the structured audit payload recorded for an operation.
type Event struct {
	ActorID      string      // User ID, or empty for anonymous requests.
	ActorMail    string      // Denormalized user email.
	ActorName    string      // Denormalized display name.
	ActorRole    string      // Role name at the time of action.
	UserAgent    string      // Truncated User-Agent header.
	Action       string      // Operation name (e.g. login, patient.view).
	Resource     string      // Target resource type (e.g. patient, user).
	ResourceName string      // Denormalized human-readable resource name.
	Result       AuditResult // AuditResultSuccess or AuditResultFailure.
	IP           string
	RequestID    string
	Detail       string
	ClinicID     string
}

// EventRow is a stored audit event with its cursor id.
type EventRow struct {
	ID         int64
	CreatedAt  time.Time
	ActorID    *string
	ActorEmail *string
	Action     string
	Resource   string
	Result     string
	Detail     *string
}

// StoredEntry is a raw stored audit log row for signature verification.
type StoredEntry struct {
	ID           int64
	CreatedAt    time.Time
	ActorID      *string
	ActorEmail   *string
	ActorName    string
	ActorRole    string
	UserAgent    string
	Action       string
	Resource     string
	ResourceName string
	Result       string
	IP           *string
	RequestID    *string
	Detail       *string
	Signature    string
	ClinicID     *string
}

// Repository defines the persistence interface for audit records.
type Repository interface {
	Recent(ctx context.Context, limit int, before int64) ([]EventRow, error)
	ForResource(ctx context.Context, resource string, limit int) ([]EventRow, error)
	LastSignature(ctx context.Context) (string, error)
	Record(ctx context.Context, ev Event, createdAt time.Time, signature string) error
	All(ctx context.Context) ([]StoredEntry, error)
}

// Logger persists audit events to the database.
type Logger struct {
	repo Repository
	log  *slog.Logger
}

// NewLogger is the Fx provider.
func NewLogger(repo Repository, log *slog.Logger) (*Logger, error) {
	if repo == nil {
		return nil, errors.New("audit: repository is nil")
	}
	return &Logger{repo: repo, log: log}, nil
}

// Recent returns the newest audit events, strictly before the cursor
// id when paging. Passing before <= 0 starts from the head of the trail.
func (l *Logger) Recent(ctx context.Context, limit int, before int64) ([]EventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	return l.repo.Recent(ctx, limit, before)
}

// ForResource returns the recent audit trail for a specific resource,
// newest first.
func (l *Logger) ForResource(ctx context.Context, resource string, limit int) ([]EventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	return l.repo.ForResource(ctx, resource, limit)
}

// Record appends an audit event to the log. Persistence errors are
// logged and swallowed so that auditing never breaks the audited
// operation.
func (l *Logger) Record(ctx context.Context, ev Event) {
	if !ev.Result.Valid() {
		l.log.Error("audit record failed",
			"action", ev.Action, "resource", ev.Resource, "result", ev.Result,
			"error", "invalid result value")
		return
	}

	createdAt := time.Now().UTC()
	if ev.ClinicID == "" {
		if id, ok := clinicctx.ClinicID(ctx); ok {
			ev.ClinicID = id.String()
		}
	}
	prev, err := l.repo.LastSignature(ctx)
	if err != nil {
		l.log.Error("audit record failed", "action", ev.Action, "error", err)
		return
	}

	signature := chainHash(prev, ev, createdAt)
	if err := l.repo.Record(ctx, ev, createdAt, signature); err != nil {
		l.log.Error("audit record failed", "action", ev.Action, "resource", ev.Resource, "error", err)
	}
}

// VerifyChain recomputes the hash chain over the whole trail and returns
// the id of the first entry whose hash does not match, or zero when the
// chain is intact. A broken chain means an entry was modified or
// reordered after it was written.
func (l *Logger) VerifyChain(ctx context.Context) (int64, error) {
	rows, err := l.repo.All(ctx)
	if err != nil {
		return 0, fmt.Errorf("audit: verify chain: %w", err)
	}
	var prev string
	for _, r := range rows {
		ev := Event{
			ActorID:      orEmpty(r.ActorID),
			ActorMail:    orEmpty(r.ActorEmail),
			ActorName:    r.ActorName,
			ActorRole:    r.ActorRole,
			UserAgent:    r.UserAgent,
			Action:       r.Action,
			Resource:     r.Resource,
			ResourceName: r.ResourceName,
			Result:       AuditResult(r.Result),
			IP:           orEmpty(r.IP),
			RequestID:    orEmpty(r.RequestID),
			Detail:       orEmpty(r.Detail),
			ClinicID:     orEmpty(r.ClinicID),
		}
		want := chainHash(prev, ev, r.CreatedAt)
		if r.Signature != want {
			return int64(r.ID), nil
		}
		prev = r.Signature
	}
	return 0, nil
}

func esc(v string) string {
	return strings.ReplaceAll(v, "|", "\\|")
}

func chainPayload(ev Event, createdAt time.Time) string {
	return strings.Join([]string{
		createdAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		esc(ev.ActorID),
		esc(ev.ActorMail),
		esc(ev.ActorName),
		esc(ev.ActorRole),
		esc(ev.UserAgent),
		esc(ev.Action),
		esc(ev.Resource),
		esc(ev.ResourceName),
		esc(ev.Result.String()),
		esc(ev.IP),
		esc(ev.RequestID),
		esc(ev.Detail),
		esc(ev.ClinicID),
	}, "|")
}

func chainHash(prev string, ev Event, createdAt time.Time) string {
	var key []byte
	if prev != "" {
		decoded, err := hex.DecodeString(prev)
		if err == nil && len(decoded) == 32 {
			key = decoded
		}
	}
	h, err := blake2b.New256(key)
	if err != nil {
		panic(fmt.Sprintf("audit: blake2b init: %v", err))
	}
	h.Write([]byte(chainPayload(ev, createdAt)))
	return hex.EncodeToString(h.Sum(nil))
}

// ErrInvalidSignature indicates a broken hash chain on verification.
var ErrInvalidSignature = errors.New("audit: signature verification failed")

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
