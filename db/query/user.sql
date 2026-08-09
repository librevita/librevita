-- Queries for the user domain (authentication accounts).

-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_name, role)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = ? COLLATE NOCASE
LIMIT 1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = ?
LIMIT 1;

-- name: ListRecentUsers :many
SELECT display_name, email, role, created_at
FROM users
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: CountUsers :one
SELECT COUNT(*)
FROM users;

-- name: CountUsersByRole :one
SELECT COUNT(*)
FROM users
WHERE role = ?;

-- name: GetMetaValue :one
SELECT value
FROM meta
WHERE key = ?
LIMIT 1;

-- name: SetMeta :exec
INSERT INTO meta (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: ListUsers :many
SELECT id, email, display_name, role, active, created_at
FROM users
WHERE (' ' || email || ' ' || display_name) LIKE '% ' || CAST(? AS TEXT) || '%'
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?;

-- name: CountUsersMatching :one
SELECT COUNT(*)
FROM users
WHERE (' ' || email || ' ' || display_name) LIKE '% ' || CAST(? AS TEXT) || '%';

-- name: UpdateUser :one
UPDATE users
SET email = ?,
display_name = ?,
role = ?,
active = ?,
updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING *;

-- name: CountActiveUsersByRole :one
SELECT COUNT(*)
FROM users
WHERE role = ? AND active = 1;

-- name: UpdateUserGuarded :one
-- Applies the update only when the guard flag is zero or more than one
-- active admin remains. Single-statement, so the last-admin check and
-- the write are atomic.
UPDATE users
SET email = ?,
    display_name = ?,
    role = ?,
    active = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE users.id = ?
  AND (CAST(? AS INTEGER) = 0 OR (SELECT COUNT(*) FROM users AS u WHERE u.role = 'admin' AND u.active = 1) > 1)
RETURNING *;

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

-- name: ListPhysicians :many
SELECT u.id, u.email, u.display_name, u.role, u.active,
       COALESCE(CAST(GROUP_CONCAT(s.name, ', ') AS TEXT), '') AS specialties
FROM users u
LEFT JOIN user_specialties us ON us.user_id = u.id
LEFT JOIN specialties s ON s.id = us.specialty_id
WHERE u.role = 'physician'
GROUP BY u.id
ORDER BY u.display_name COLLATE NOCASE;

-- name: CreateStaffChangeRequest :one
INSERT INTO staff_change_requests (id, user_id, requested_by, status, changes)
VALUES (?, ?, ?, 'pending', ?)
RETURNING *;

-- name: ListStaffChangeRequests :many
SELECT r.id, r.user_id, r.requested_by, r.status, r.changes, r.decision_note,
       r.created_at, r.decided_at, r.decided_by,
       u.email AS user_email, u.display_name AS user_name,
       req.email AS requester_email
FROM staff_change_requests r
JOIN users u ON u.id = r.user_id
JOIN users req ON req.id = r.requested_by
WHERE r.status = ?
ORDER BY r.created_at DESC
LIMIT ?;

-- name: GetStaffChangeRequest :one
SELECT *
FROM staff_change_requests
WHERE id = ?
LIMIT 1;

-- name: DecideStaffChangeRequest :exec
UPDATE staff_change_requests
SET status = ?,
    decision_note = ?,
    decided_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    decided_by = ?
WHERE id = ? AND status = 'pending';

-- name: ListStaffChangeRequestsByRequester :many
SELECT r.id, r.user_id, r.requested_by, r.status, r.changes, r.decision_note,
       r.created_at, r.decided_at, r.decided_by,
       u.email AS user_email, u.display_name AS user_name
FROM staff_change_requests r
JOIN users u ON u.id = r.user_id
WHERE r.requested_by = ?
ORDER BY r.created_at DESC
LIMIT ?;
