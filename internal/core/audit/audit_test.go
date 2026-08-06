package audit

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	_ "modernc.org/sqlite"

	"librevita.org/internal/core/database"
)

func openAuditDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:audit-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRecordPersistsEvent(t *testing.T) {
	db := openAuditDB(t)
	logger, err := NewLogger(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	logger.Record(context.Background(), Event{
		ActorID: 7, ActorMail: "ana@example.org",
		Action: "login", Resource: "user", Result: ResultSuccess,
		IP: "127.0.0.1", RequestID: "req-123", Detail: "",
	})

	var actorID int64
	var action, result, requestID string
	err = db.QueryRow(`SELECT actor_id, action, result, request_id FROM audit_log`).
		Scan(&actorID, &action, &result, &requestID)
	if err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	if actorID != 7 || action != "login" || result != ResultSuccess || requestID != "req-123" {
		t.Fatalf("unexpected row: %d %q %q %q", actorID, action, result, requestID)
	}
}

func TestRecordRequiresSQLite(t *testing.T) {
	if _, err := NewLogger(nil, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("NewLogger(nil) should fail")
	}
}

func TestRecordSwallowsWriteErrors(t *testing.T) {
	// A closed database makes the INSERT fail; Record must not panic and
	// the caller must not receive an error.
	db := openAuditDB(t)
	logger, err := NewLogger(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	logger.Record(context.Background(), Event{
		Action: "login", Resource: "user", Result: ResultFailure,
	})
}
