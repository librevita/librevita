-- Queries for the specialties domain.
-- name: ListSpecialties :many
SELECT *
FROM specialties
WHERE clinic_id = ?
ORDER BY name COLLATE NOCASE;
-- name: CreateSpecialty :one
INSERT INTO specialties (id, clinic_id, name)
VALUES (?, ?, ?)
RETURNING *;
-- name: DeleteSpecialty :exec
DELETE FROM specialties
WHERE id = ? AND clinic_id = ?;

-- name: GetSpecialtyByID :one
SELECT *
FROM specialties
WHERE id = ? AND clinic_id = ?
LIMIT 1;
-- name: ClearUserSpecialties :exec
DELETE FROM user_specialties
WHERE user_id = ?;
-- name: AddUserSpecialty :exec
INSERT INTO user_specialties (user_id, specialty_id)
VALUES (?, ?);
-- name: ListUserSpecialties :many
SELECT s.id, s.clinic_id, s.name, s.created_at
FROM specialties s
JOIN user_specialties us ON us.specialty_id = s.id
WHERE us.user_id = ?
ORDER BY s.name COLLATE NOCASE;
-- name: ListSpecialtiesPage :many
SELECT *
FROM specialties
WHERE clinic_id = ?
ORDER BY name COLLATE NOCASE
LIMIT ? OFFSET ?;
-- name: CountSpecialties :one
SELECT COUNT(*)
FROM specialties
WHERE clinic_id = ?;
