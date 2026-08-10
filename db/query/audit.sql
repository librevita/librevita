-- Queries for the audit trail. created_at uses the database default so
-- that the timestamp format stays consistent with the migrations.

-- name: CreateAuditEvent :exec
INSERT INTO audit_log (actor_id,
actor_email,
actor_name,
actor_role,
user_agent,
action,
resource,
resource_name,
result,
ip,
request_id,
detail,
created_at,
signature)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRecentAuditEvents :many
SELECT id, created_at, actor_id, actor_email, actor_name, actor_role, user_agent, action, resource, resource_name, result, detail, signature
FROM audit_log
ORDER BY id DESC
LIMIT ?;

-- name: ListAuditEventsBefore :many
SELECT id, created_at, actor_id, actor_email, actor_name, actor_role, user_agent, action, resource, resource_name, result, detail, signature
FROM audit_log
WHERE id < ?
ORDER BY id DESC
LIMIT ?;

-- name: ListAuditEventsForResource :many
SELECT id, created_at, actor_id, actor_email, actor_name, actor_role, user_agent, action, resource, resource_name, result, detail, signature
FROM audit_log
WHERE resource = ?
ORDER BY id DESC
LIMIT ?;

-- name: GetLastAuditSignature :one
SELECT signature
FROM audit_log
ORDER BY id DESC
LIMIT 1;

-- name: ListAuditChain :many
SELECT id, created_at, actor_id, actor_email, actor_name, actor_role, user_agent, action, resource, resource_name, result, ip, request_id, detail, signature
FROM audit_log
ORDER BY id ASC;
