-- +goose Up
-- +goose NO TRANSACTION
-- Dynamic CEL authorization policies. The primary key is a UUIDv7 generated
-- by the application; name is the stable permission identifier referenced
-- by routes. Defaults are seeded from internal/core/policy/policy.go on
-- startup; the stored expression always wins over the default. Edited by
-- the admin panel (/admin/policies).

CREATE TABLE policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    expression TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE policies;
