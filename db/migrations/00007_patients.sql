-- +goose Up
-- +goose NO TRANSACTION
-- Patients: the core clinical record of the clinic.
CREATE TABLE patients (
    id TEXT PRIMARY KEY,
    clinic_id TEXT NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    birth_date TEXT,
    sex TEXT NOT NULL DEFAULT 'unknown'
    CHECK (sex IN ('female', 'male', 'other', 'unknown')),
    document TEXT,
    phone TEXT,
    email TEXT,
    street TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'inactive')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_patients_clinic ON patients (clinic_id);
CREATE INDEX idx_patients_name ON patients (display_name COLLATE NOCASE);
CREATE UNIQUE INDEX idx_patients_clinic_document
ON patients (clinic_id, document)
WHERE document IS NOT NULL;
