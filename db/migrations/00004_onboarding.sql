-- +goose Up
-- +goose NO TRANSACTION
-- Clinic profile created during onboarding (owned by the clinic domain)
-- and the system metadata table.

CREATE TABLE clinics (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    tax_id      TEXT,
    phone       TEXT,
    email       TEXT,
    street      TEXT,
    city        TEXT,
    state       TEXT,
    postal_code TEXT,
    country     TEXT NOT NULL DEFAULT 'BR',
    timezone    TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE TABLE meta (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE meta;
DROP TABLE clinics;
