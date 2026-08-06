-- +goose Up
-- +goose NO TRANSACTION
-- Change history for dynamic CEL policies. Every Set() writes the new
-- expression together with the acting user inside the same transaction;
-- the policies table holds only the current expression.
-- origin records where a version came from: 'seed' (startup defaults),
-- 'admin' (admin panel), or 'system'. Versions are never deleted; the
-- foreign key restricts policy deletion so that history survives.

CREATE TABLE policy_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    policy_id TEXT NOT NULL REFERENCES policies(id) ON DELETE RESTRICT,
    expression TEXT NOT NULL,
    changed_by TEXT,
    changed_by_email TEXT,
    origin TEXT NOT NULL DEFAULT 'system'
    CHECK (origin IN ('seed', 'admin', 'system')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_policy_versions_policy ON policy_versions (policy_id, id DESC);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE policy_versions;
