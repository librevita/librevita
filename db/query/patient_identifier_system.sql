-- Queries for identifier_systems: the administrator-defined registry
-- of document types (FHIR Identifier systems). Like roles, systems are
-- rows, not code -- a new jurisdiction registers a pattern and
-- transformation without a deployment.

-- name: ListIdentifierSystems :many
SELECT *
FROM identifier_systems
ORDER BY system;

-- name: ListActiveIdentifierSystems :many
SELECT *
FROM identifier_systems
WHERE active = 1
ORDER BY length(pattern) DESC, system;

-- name: GetIdentifierSystemByID :one
SELECT *
FROM identifier_systems
WHERE id = ?
LIMIT 1;

-- name: CreateIdentifierSystem :one
INSERT INTO identifier_systems (
    id, system, display_name, pattern, transform,
    check_algorithm,
    check_base_len,
    check_dv_count,
    check_start_weight,
    created_by
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateIdentifierSystem :one
UPDATE identifier_systems
SET
    display_name = ?,
    pattern = ?,
    transform = ?,
    check_algorithm = ?,
    check_base_len = ?,
    check_dv_count = ?,
    check_start_weight = ?,
    active = ?,
    updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?
RETURNING *;
