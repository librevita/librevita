package audit_test

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/database"
	"librevita.org/internal/types"
)

func openAuditTest(t *testing.T) (*sql.DB, audit.Repository) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:audit-test?mode=memory&cache=shared&_time_format=sqlite")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	repo := audit.NewAuditRepository(client)
	return db, repo
}

func TestRecordPersistsEvent(t *testing.T) {
	db, repo := openAuditTest(t)
	logger, err := audit.NewLogger(repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	logger.Record(context.Background(), audit.Event{
		ActorID: "01990000-0000-7000-8000-000000000001", ActorMail: "ana@example.org",
		Action: "login", Resource: "user", Result: types.AuditResultSuccess,
		IP: "127.0.0.1", RequestID: "req-123", Detail: "",
	})

	var actorID string
	var action, result, requestID string
	err = db.QueryRow(`SELECT actor_id, action, result, request_id FROM audit_log`).
		Scan(&actorID, &action, &result, &requestID)
	if err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	if actorID != "01990000-0000-7000-8000-000000000001" || action != "login" || result != types.AuditResultSuccess.String() || requestID != "req-123" {
		t.Fatalf("unexpected row: %q %q %q %q", actorID, action, result, requestID)
	}
}

func TestRecordRequiresRepository(t *testing.T) {
	if _, err := audit.NewLogger(nil, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("NewLogger(nil) should fail")
	}
}

func TestRecordSwallowsWriteErrors(t *testing.T) {
	db, repo := openAuditTest(t)
	logger, err := audit.NewLogger(repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	logger.Record(context.Background(), audit.Event{
		Action: "login", Resource: "user", Result: types.AuditResultFailure,
	})
}

func TestHashChain(t *testing.T) {
	db, repo := openAuditTest(t)
	logger := slog.New(slog.DiscardHandler)
	l, err := audit.NewLogger(repo, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, ev := range []audit.Event{
		{Action: "login", Resource: "user", Result: types.AuditResultSuccess},
		{Action: "patient.update", Resource: "patient:1", Result: types.AuditResultSuccess, Detail: "phone changed"},
		{Action: "authorize", Resource: "policy:admin.view", Result: types.AuditResultFailure},
	} {
		l.Record(ctx, ev)
	}
	if broken, err := l.VerifyChain(ctx); err != nil || broken != 0 {
		t.Fatalf("intact chain: broken=%d err=%v", broken, err)
	}

	// Every row must carry a hash derived from the previous one.
	rows, err := l.Recent(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}

	// Forging an entry with a wrong hash breaks the chain from there on.
	if _, err := db.Exec(`INSERT INTO audit_log (actor_name, actor_role, user_agent, action, resource, resource_name, result, created_at, signature)
		VALUES ('', '', '', 'forged', 'user', '', 'success', '2026-01-01T00:00:00.000Z', 'deadbeef')`); err != nil {
		t.Fatal(err)
	}
	if broken, err := l.VerifyChain(ctx); err != nil || broken == 0 {
		t.Fatalf("broken chain must be detected, broken=%d err=%v", broken, err)
	}
}

func TestAuditLogAppendOnly(t *testing.T) {
	db, repo := openAuditTest(t)
	l, err := audit.NewLogger(repo, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	l.Record(ctx, audit.Event{Action: "login", Resource: "user", Result: types.AuditResultSuccess})

	if _, err := db.Exec(`UPDATE audit_log SET detail = 'tampered'`); err == nil {
		t.Fatal("UPDATE on audit_log must be refused")
	}
	if _, err := db.Exec(`DELETE FROM audit_log`); err == nil {
		t.Fatal("DELETE on audit_log must be refused")
	}
}
