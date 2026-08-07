-- Queries for the patient domain. Timestamps use the database default so
-- the format stays consistent with the migrations.

-- name: CreatePatient :one
INSERT INTO patients (
    id, clinic_id, display_name, birth_date, sex, document,
    phone, email, street, city, state, postal_code, notes, created_by
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPatientWithCreator :one
SELECT p.*, u.email AS created_by_email
FROM patients p
LEFT JOIN users u ON u.id = p.created_by
WHERE p.id = ?
LIMIT 1;

-- name: GetPatientByID :one
SELECT *
FROM patients
WHERE id = ?
LIMIT 1;

-- name: ListPatients :many
SELECT *
FROM patients
WHERE clinic_id = ?
  AND (@status_empty = '' OR status = @status_filter)
  AND (@after_empty = '' OR display_name > @after COLLATE NOCASE)
ORDER BY display_name COLLATE NOCASE
LIMIT @limit;

-- name: SearchPatients :many
SELECT *
FROM patients
WHERE clinic_id = ?
  AND (@status_empty = '' OR status = @status_filter)
  AND (@after_empty = '' OR display_name > @after COLLATE NOCASE)
  AND (display_name LIKE ? COLLATE NOCASE
       OR document LIKE ?
       OR email LIKE ? COLLATE NOCASE)
ORDER BY display_name COLLATE NOCASE
LIMIT @limit;

-- name: UpdatePatient :one
UPDATE patients
SET display_name = ?,
birth_date = ?,
sex = ?,
document = ?,
phone = ?,
email = ?,
street = ?,
city = ?,
state = ?,
postal_code = ?,
notes = ?,
updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?
RETURNING *;

-- name: UpdatePatientStatus :exec
UPDATE patients
SET status = ?, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?;

-- name: CountPatients :one
SELECT COUNT(*)
FROM patients
WHERE clinic_id = ?;

-- name: PatientDocumentExists :one
SELECT EXISTS(
    SELECT 1
    FROM patients
    WHERE clinic_id = ? AND document = ? AND id <> ?
);
