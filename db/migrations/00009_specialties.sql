-- +goose Up
-- +goose NO TRANSACTION
-- Clinician specialties (psychologist, physiotherapist, ...) managed by
-- the administrator, and the many-to-many mapping to user accounts so a
-- clinician can have more than one specialty.

CREATE TABLE specialties (
    id TEXT PRIMARY KEY,
    clinic_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX idx_specialties_clinic_name
ON specialties (clinic_id, name COLLATE NOCASE);

CREATE TABLE user_specialties (
user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
specialty_id TEXT NOT NULL REFERENCES specialties(id) ON DELETE CASCADE,
PRIMARY KEY (user_id, specialty_id)
);

CREATE INDEX idx_user_specialties_specialty ON user_specialties (specialty_id);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE user_specialties;
DROP TABLE specialties;
