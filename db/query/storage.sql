-- Master index of stored files: metadata for the blobs managed by
-- internal/core/storage. The blob store is addressed by key; these
-- queries make the metadata findable by owning domain and entity.

-- name: CreateStorageObject :one
INSERT INTO storage_objects (
    id,
    key,
    domain,
    resource_id,
    original_name,
    content_type,
    size,
    etag,
    checksum,
    created_by
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetStorageObjectByKey :one
SELECT * FROM storage_objects
WHERE key = ?
LIMIT 1;

-- name: GetStorageObjectByID :one
SELECT * FROM storage_objects
WHERE id = ?
LIMIT 1;

-- name: ListStorageObjectsByResource :many
SELECT * FROM storage_objects
WHERE domain = ? AND resource_id = ?
ORDER BY created_at DESC;

-- name: DeleteStorageObject :exec
DELETE FROM storage_objects
WHERE id = ?;

-- name: DeleteStorageObjectsByResource :exec
DELETE FROM storage_objects
WHERE domain = ? AND resource_id = ?;

-- name: GetStorageObjectByResourceAndID :one
-- Belonging check: resolves an object only when it belongs to the
-- given domain and resource, so a bare object id is never enough to
-- reach a file (IDOR protection).
SELECT * FROM storage_objects
WHERE domain = ? AND resource_id = ? AND id = ?
LIMIT 1;
