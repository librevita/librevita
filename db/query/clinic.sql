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

-- GetClinic resolves the installation's single clinic (see
-- internal/domain/clinic/provider.go for the single-clinic decision).
-- The ORDER BY makes the LIMIT 1 deterministic should a second row
-- ever appear.
-- name: GetClinic :one
SELECT *
FROM clinics
ORDER BY created_at
LIMIT 1;

-- name: UpdateClinicTimezone :exec
UPDATE clinics
SET timezone = ?, updated_at = (STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?;
