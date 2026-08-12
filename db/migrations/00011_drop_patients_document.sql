-- +goose Up
-- +goose NO TRANSACTION
-- The legacy patients.document column held the identification document
-- in plaintext. Identification documents are now FHIR-style identifiers
-- stored encrypted at field level in patient_identifiers (see
-- 00010_patient_identifiers.sql); a plaintext twin column would be a
-- second, unprotected copy of the same data. The column and its unique
-- index are dropped; SQLite requires the index to go first.

DROP INDEX idx_patients_clinic_document;

ALTER TABLE patients DROP COLUMN document;

-- +goose Down
-- +goose NO TRANSACTION
-- The down migration recreates the column: the encrypted identifiers
-- are the canonical copy, so nothing can be restored from it.
ALTER TABLE patients ADD COLUMN document TEXT;

CREATE UNIQUE INDEX idx_patients_clinic_document
ON patients (clinic_id, document)
WHERE document IS NOT NULL;
