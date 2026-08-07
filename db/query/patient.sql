-- Queries for the patient domain. Timestamps use the database default so
-- the format stays consistent with the migrations.

-- name: CreatePatient :one
INSERT INTO patients (
    id, clinic_id, display_name, birth_date, sex, document,
    phone, email, street, city, state, postal_code, notes
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPatientByID :one
SELECT *
FROM patients
WHERE id = ?
LIMIT 1;

-- name: ListPatients :many
SELECT *
FROM patients
WHERE clinic_id = ?
ORDER BY display_name COLLATE NOCASE
LIMIT ?;

-- name: SearchPatients :many
SELECT *
FROM patients
WHERE clinic_id = ?
AND (display_name LIKE ? COLLATE NOCASE
OR document LIKE ?
OR email LIKE ? COLLATE NOCASE)
ORDER BY display_name COLLATE NOCASE
LIMIT ?;

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
