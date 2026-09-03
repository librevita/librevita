package audit_test

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/database"
	"librevita.org/internal/database/record"
	"librevita.org/pkg/log"
)

func openAuditTest(t *testing.T) (*sql.DB, audit.Repository) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:audit-test?mode=memory&cache=shared&_time_format=sqlite")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	err = database.Migrate(context.Background(), db, log.Nop())
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := record.NewClient(record.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	repo := audit.NewAuditRepository(client)
	return db, repo
}

func TestRecordPersistsEvent(t *testing.T) {
	db, repo := openAuditTest(t)
	logger, err := audit.NewLogger(repo, log.Nop())
	require.NoError(t, err)

	logger.Record(context.Background(), audit.Event{
		ActorID: "01990000-0000-7000-8000-000000000001", ActorMail: "ana@example.org",
		Action: "login", Resource: "user", Result: audit.AuditResultSuccess,
		IP: "127.0.0.1", RequestID: "req-123", Detail: "",
	})

	var actorID string
	var action, result, requestID string
	err = db.QueryRow(`SELECT actor_id, action, result, request_id FROM audit_log`).
		Scan(&actorID, &action, &result, &requestID)
	require.NoError(t, err)
	assert.Equal(t, "01990000-0000-7000-8000-000000000001", actorID)
	assert.Equal(t, "login", action)
	assert.Equal(t, audit.AuditResultSuccess.String(), result)
	assert.Equal(t, "req-123", requestID)
}

func TestRecordRequiresRepository(t *testing.T) {
	_, err := audit.NewLogger(nil, log.Nop())
	assert.Error(t, err)
}

func TestRecordSwallowsWriteErrors(t *testing.T) {
	db, repo := openAuditTest(t)
	logger, err := audit.NewLogger(repo, log.Nop())
	require.NoError(t, err)
	db.Close()

	logger.Record(context.Background(), audit.Event{
		Action: "login", Resource: "user", Result: audit.AuditResultFailure,
	})
}

func TestHashChain(t *testing.T) {
	db, repo := openAuditTest(t)
	logger := log.Nop()
	l, err := audit.NewLogger(repo, logger)
	require.NoError(t, err)
	ctx := context.Background()

	for _, ev := range []audit.Event{
		{Action: "login", Resource: "user", Result: audit.AuditResultSuccess},
		{Action: "patient.update", Resource: "patient:1", Result: audit.AuditResultSuccess, Detail: "phone changed"},
		{Action: "authorize", Resource: "policy:admin.view", Result: audit.AuditResultFailure},
	} {
		l.Record(ctx, ev)
	}
	broken, err := l.VerifyChain(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), broken)

	// Every row must carry a hash derived from the previous one.
	rows, err := l.Recent(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, rows, 3)

	// Forging an entry with a wrong hash breaks the chain from there on.
	_, err = db.Exec(`INSERT INTO audit_log (actor_name, actor_role, user_agent, action, resource, resource_name, result, created_at, signature)
		VALUES ('', '', '', 'forged', 'user', '', 'success', '2026-01-01T00:00:00.000Z', 'deadbeef')`)
	require.NoError(t, err)

	broken, err = l.VerifyChain(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, int64(0), broken)
}

func TestAuditLogAppendOnly(t *testing.T) {
	db, repo := openAuditTest(t)
	l, err := audit.NewLogger(repo, log.Nop())
	require.NoError(t, err)
	ctx := context.Background()
	l.Record(ctx, audit.Event{Action: "login", Resource: "user", Result: audit.AuditResultSuccess})

	_, err = db.Exec(`UPDATE audit_log SET detail = 'tampered'`)
	assert.Error(t, err, "UPDATE on audit_log must be refused")

	_, err = db.Exec(`DELETE FROM audit_log`)
	assert.Error(t, err, "DELETE on audit_log must be refused")
}

func TestForResourceAndPagination(t *testing.T) {
	_, repo := openAuditTest(t)
	l, err := audit.NewLogger(repo, log.Nop())
	require.NoError(t, err)
	ctx := context.Background()

	l.Record(ctx, audit.Event{Action: "patient.view", Resource: "patient:10", Result: audit.AuditResultSuccess})
	l.Record(ctx, audit.Event{Action: "patient.update", Resource: "patient:10", Result: audit.AuditResultSuccess})
	l.Record(ctx, audit.Event{Action: "patient.view", Resource: "patient:20", Result: audit.AuditResultSuccess})

	// 1. ForResource
	res10, err := l.ForResource(ctx, "patient:10", 10)
	require.NoError(t, err)
	assert.Len(t, res10, 2)

	// 2. Recent with limit <= 0 default
	recentDefault, err := l.Recent(ctx, 0, 0)
	require.NoError(t, err)
	assert.Len(t, recentDefault, 3)

	// 3. Recent with before cursor
	beforeID := recentDefault[0].ID
	recentPaged, err := l.Recent(ctx, 10, beforeID)
	require.NoError(t, err)
	assert.Len(t, recentPaged, 2)
}
