-- +goose Up
-- +goose NO TRANSACTION
-- Onboarding: clinic profile and system metadata.
-- The clinic is created together with the initial admin account by the
-- setup flow (internal/domain/user/usecase). The meta table stores the
-- setup_completed marker so that setup can run exactly once, even if every
-- account or the clinic are later removed.

CREATE TABLE clinics (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    tax_id TEXT,
    phone TEXT,
    email TEXT,
    street TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    country TEXT NOT NULL DEFAULT 'BR',
    timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE meta;
DROP TABLE clinics;
