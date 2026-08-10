-- Queries for the roles domain.
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
