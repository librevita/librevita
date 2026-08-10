-- +goose Up
-- +goose NO TRANSACTION
-- Durable audit trail. Every entry records who did what, when, and with
-- what outcome. Passwords, tokens, and CSRF values are never stored.
-- actor_id holds the UUIDv7 user id and is nullable for anonymous events.
-- The table is owned by internal/core/audit and is intentionally absent
-- from db/schema/schema.sql (no sqlc queries).

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id    TEXT,
    actor_email TEXT,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    result      TEXT NOT NULL CHECK (result IN ('success', 'failure')),
    ip          TEXT,
    request_id  TEXT,
    detail      TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_audit_log_actor ON audit_log(actor_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE audit_log;
