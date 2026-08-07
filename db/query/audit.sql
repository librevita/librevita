-- Queries for the audit trail. created_at uses the database default so
-- that the timestamp format stays consistent with the migrations.

-- name: CreateAuditEvent :exec
INSERT INTO audit_log (actor_id,
actor_email,
action,
resource,
result,
ip,
request_id,
detail)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRecentAuditEvents :many
SELECT id, created_at, actor_id, actor_email, action, resource, result, detail
FROM audit_log
ORDER BY id DESC
LIMIT ?;

-- name: ListAuditEventsBefore :many
SELECT id, created_at, actor_id, actor_email, action, resource, result, detail
FROM audit_log
WHERE id < ?
ORDER BY id DESC
LIMIT ?;

-- name: ListAuditEventsForResource :many
SELECT id, created_at, actor_id, actor_email, action, resource, result, detail
FROM audit_log
WHERE resource = ?
ORDER BY id DESC
LIMIT ?;
