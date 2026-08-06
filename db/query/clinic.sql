-- Queries for the clinic domain (onboarding profile).

-- name: CreateClinic :one
INSERT INTO clinics (
    id,
    name,
    tax_id,
    phone,
    email,
    street,
    city,
    state,
    postal_code,
    country,
    timezone
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CountClinics :one
SELECT COUNT(*)
FROM clinics;

-- name: GetClinic :one
SELECT *
FROM clinics
LIMIT 1;

-- name: UpdateClinicTimezone :exec
UPDATE clinics
SET timezone = ?, updated_at = (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?;
