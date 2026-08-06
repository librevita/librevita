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
