-- +goose Up
-- +goose NO TRANSACTION
-- Clinician specialties and their many-to-many mapping to user accounts,
-- so a physician can carry more than one (psychologist, physiotherapist,
-- ...). Owned by the user domain.

CREATE TABLE specialties (
    id         TEXT PRIMARY KEY,
    clinic_id  TEXT NOT NULL,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE UNIQUE INDEX idx_specialties_clinic_name
    ON specialties (clinic_id, name COLLATE NOCASE);

CREATE TABLE user_specialties (
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    specialty_id TEXT NOT NULL REFERENCES specialties(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, specialty_id)
) STRICT;

CREATE INDEX idx_user_specialties_specialty ON user_specialties (specialty_id);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE user_specialties;
DROP TABLE specialties;
