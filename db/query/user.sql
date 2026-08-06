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

-- name: CountUsers :one
SELECT COUNT(*)
FROM users;

-- name: GetMetaValue :one
SELECT value
FROM meta
WHERE key = ?
LIMIT 1;

-- name: SetMeta :exec
INSERT INTO meta (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
