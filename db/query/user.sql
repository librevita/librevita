-- Queries for the user domain (authentication accounts).

-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_name, role_id)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT u.id, u.email, u.password_hash, u.display_name, u.active,
       u.created_at, u.updated_at, u.role_id, r.name AS role_name,
       r.is_clinical AS role_is_clinical
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE u.email = ? COLLATE NOCASE
LIMIT 1;

-- name: GetUserByID :one
SELECT u.id, u.email, u.password_hash, u.display_name, u.active,
       u.created_at, u.updated_at, u.role_id, r.name AS role_name,
       r.is_clinical AS role_is_clinical
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE u.id = ?
LIMIT 1;

-- name: ListRecentUsers :many
SELECT u.display_name, u.email, r.name AS role_name, u.created_at
FROM users u
JOIN roles r ON r.id = u.role_id
ORDER BY u.created_at DESC, u.id DESC
LIMIT ?;

-- name: CountUsers :one
SELECT COUNT(*)
FROM users;

-- name: CountUsersByRole :one
SELECT COUNT(*)
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE r.name = ? COLLATE NOCASE;

-- name: CountActiveUsersByRole :one
SELECT COUNT(*)
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE r.name = ? COLLATE NOCASE AND u.active = 1;

-- name: ListUsers :many
SELECT u.id, u.email, u.display_name, u.active, u.created_at,
       r.name AS role_name
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE (' ' || u.email || ' ' || u.display_name) LIKE '% ' || CAST(? AS TEXT) || '%'
ORDER BY u.created_at DESC, u.id DESC
LIMIT ? OFFSET ?;

-- name: CountUsersMatching :one
SELECT COUNT(*)
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE (' ' || u.email || ' ' || u.display_name) LIKE '% ' || CAST(? AS TEXT) || '%';

-- name: UpdateUser :one
UPDATE users
SET email = ?,
    display_name = ?,
    role_id = ?,
    active = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING *;

-- name: UpdateUserGuarded :one
-- Applies the update only when the guard flag is zero or more than one
-- active admin remains. Single-statement, so the last-admin check and
-- the write are atomic.
UPDATE users
SET email = ?,
    display_name = ?,
    role_id = ?,
    active = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE users.id = ?
  AND (CAST(? AS INTEGER) = 0 OR (SELECT COUNT(*) FROM users AS u JOIN roles AS r ON r.id = u.role_id WHERE r.name = 'admin' AND u.active = 1) > 1)
RETURNING *;

-- name: ListPhysicians :many
SELECT u.id, u.email, u.display_name, u.active,
       COALESCE(CAST(GROUP_CONCAT(s.name, ', ') AS TEXT), '') AS specialties
FROM users u
JOIN roles r ON r.id = u.role_id
LEFT JOIN user_specialties us ON us.user_id = u.id
LEFT JOIN specialties s ON s.id = us.specialty_id
WHERE r.is_clinical = 1
GROUP BY u.id
ORDER BY u.display_name COLLATE NOCASE;

-- name: CreateStaffChangeRequest :one
INSERT INTO staff_change_requests (id, user_id, requested_by, status, changes)
VALUES (?, ?, ?, 'pending', ?)
RETURNING *;

-- name: ListStaffChangeRequestsFiltered :many
SELECT r.id, r.user_id, r.requested_by, r.status, r.changes, r.decision_note,
       r.created_at, r.decided_at, r.decided_by,
       u.email AS user_email, u.display_name AS user_name,
       req.email AS requester_email,
       dec.email AS decided_by_email
FROM staff_change_requests r
JOIN users u ON u.id = r.user_id
JOIN users req ON req.id = r.requested_by
LEFT JOIN users dec ON dec.id = r.decided_by
WHERE (@status_empty = '' OR r.status = @status_filter)
  AND (@q_empty = '' OR (u.display_name || ' ' || u.email || ' ' || req.email || ' ' || r.changes) LIKE @q COLLATE NOCASE)
ORDER BY r.created_at DESC
LIMIT ? OFFSET ?;

-- name: CountStaffChangeRequestsFiltered :one
SELECT COUNT(*)
FROM staff_change_requests r
JOIN users u ON u.id = r.user_id
JOIN users req ON req.id = r.requested_by
WHERE (@status_empty = '' OR r.status = @status_filter)
  AND (@q_empty = '' OR (u.display_name || ' ' || u.email || ' ' || req.email || ' ' || r.changes) LIKE @q COLLATE NOCASE);

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

-- name: ListRoles :many
SELECT *
FROM roles
ORDER BY system DESC, name COLLATE NOCASE;

-- name: GetRoleByName :one
SELECT *
FROM roles
WHERE name = ? COLLATE NOCASE
LIMIT 1;

-- name: GetRoleByID :one
SELECT *
FROM roles
WHERE id = ?
LIMIT 1;

-- name: CreateRole :one
INSERT INTO roles (id, name, system, is_clinical)
VALUES (?, ?, 0, ?)
RETURNING *;

-- name: RenameRole :one
UPDATE roles
SET name = ?
WHERE id = ? AND system = 0
RETURNING *;

-- name: SetRoleClinical :one
UPDATE roles
SET is_clinical = ?
WHERE id = ? AND system = 0
RETURNING *;

-- name: DeleteRole :exec
DELETE FROM roles
WHERE roles.id = ? AND roles.system = 0
  AND NOT EXISTS (SELECT 1 FROM users WHERE users.role_id = roles.id);

-- name: CountUsersByRoleID :one
SELECT COUNT(*)
FROM users
WHERE role_id = ?;

-- name: GetMetaValue :one
SELECT value
FROM meta
WHERE key = ?
LIMIT 1;

-- name: SetMeta :exec
INSERT INTO meta (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

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

-- name: ListPhysiciansPage :many
SELECT u.id, u.email, u.display_name, u.active,
       COALESCE(CAST(GROUP_CONCAT(s.name, ', ') AS TEXT), '') AS specialties
FROM users u
JOIN roles r ON r.id = u.role_id
LEFT JOIN user_specialties us ON us.user_id = u.id
LEFT JOIN specialties s ON s.id = us.specialty_id
WHERE r.is_clinical = 1
GROUP BY u.id
ORDER BY u.display_name COLLATE NOCASE
LIMIT ? OFFSET ?;

-- name: CountPhysicians :one
SELECT COUNT(*)
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE r.is_clinical = 1;

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
