-- +goose Up
-- +goose NO TRANSACTION
-- Authentication and authorization. The account role is a row in roles
-- (relational and dynamic) instead of a fixed TEXT CHECK enum; the four
-- original roles are seeded as system roles: they cannot be renamed or
-- deleted, because the CEL policies reference them by name.

CREATE TABLE roles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE COLLATE NOCASE,
    system      INTEGER NOT NULL DEFAULT 0 CHECK (system IN (0, 1)),
    is_clinical INTEGER NOT NULL DEFAULT 0 CHECK (is_clinical IN (0, 1)),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

INSERT INTO roles (id, name, system, is_clinical) VALUES
    ('00000000-0000-7000-8000-000000000001', 'admin', 1, 0),
    ('00000000-0000-7000-8000-000000000002', 'physician', 1, 1),
    ('00000000-0000-7000-8000-000000000003', 'receptionist', 1, 0),
    ('00000000-0000-7000-8000-000000000004', 'patient', 1, 0);

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role_id       TEXT NOT NULL REFERENCES roles(id),
    active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_users_role ON users (role_id);

-- Server-side session revocation index, owned by internal/core/auth.
CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE roles;
