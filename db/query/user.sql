-- Queries for the user domain.
-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_name, role_id)
VALUES (?, ?, ?, ?, ?)
RETURNING *;
-- name: GetUserByEmail :one
SELECT u.id, u.email, u.password_hash, u.display_name, u.active,
       u.timezone, u.ui_theme, u.created_at, u.updated_at, u.role_id,
       r.name AS role_name, r.is_clinical AS role_is_clinical
FROM users u
JOIN roles r ON r.id = u.role_id
WHERE u.email = ? COLLATE NOCASE
LIMIT 1;
-- name: GetUserByID :one
SELECT u.id, u.email, u.password_hash, u.display_name, u.active,
       u.timezone, u.ui_theme, u.created_at, u.updated_at, u.role_id,
       r.name AS role_name, r.is_clinical AS role_is_clinical
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

-- name: UpdateUserPreferences :one
-- Stores the user's UI theme and personal timezone. The timezone may be
-- empty, which means "inherit the clinic timezone".
UPDATE users
SET timezone = ?,
    ui_theme = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING *;
