-- +goose Up
-- +goose NO TRANSACTION
-- Durable audit trail. Every entry records who did what, when, and with
-- what outcome. Passwords, tokens, and CSRF values are never stored.
-- actor_id holds the UUIDv7 user id and is nullable for anonymous events.
-- The table is owned by internal/core/audit and is intentionally absent
-- from db/schema/schema.sql (no sqlc queries).

CREATE TABLE audit_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id      TEXT,
    actor_email   TEXT,
    action        TEXT NOT NULL,
    resource      TEXT NOT NULL,
    result        TEXT NOT NULL CHECK (result IN ('success', 'failure')),
    ip            TEXT,
    request_id    TEXT,
    detail        TEXT,
    actor_name    TEXT NOT NULL,
    actor_role    TEXT NOT NULL,
    user_agent    TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    signature     TEXT NOT NULL
) STRICT;

-- Layer 1: the trail is append-only. Any UPDATE or DELETE raises ABORT,
-- so entries can only be created. Layer 2: the signature column holds
-- the BLAKE2b digest of the previous signature plus the row payload,
-- computed by the application (see internal/core/audit).
-- so entries can only be created.
-- +goose StatementBegin
CREATE TRIGGER audit_log_no_update
BEFORE UPDATE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER audit_log_no_delete
BEFORE DELETE ON audit_log
BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only');
END;
-- +goose StatementEnd

CREATE INDEX idx_audit_log_actor ON audit_log(actor_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE audit_log;
DROP TRIGGER audit_log_no_update;
DROP TRIGGER audit_log_no_delete;
