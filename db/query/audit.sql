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
