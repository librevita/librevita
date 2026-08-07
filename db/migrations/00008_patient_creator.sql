-- +goose Up
-- +goose NO TRANSACTION
-- Track who registered each patient. Existing rows are backfilled from
-- the audit trail when the create event is still present.
ALTER TABLE patients ADD COLUMN created_by TEXT REFERENCES users(id);

UPDATE patients
SET
    created_by = (
        SELECT actor_id
        FROM audit_log
        WHERE action = 'patient.create' AND resource = 'patient:' || patients.id
        LIMIT 1
    )
WHERE created_by IS NULL;
