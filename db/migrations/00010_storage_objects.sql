-- +goose Up
-- +goose NO TRANSACTION
-- Master index of stored files. The object store (local directory or
-- S3-compatible API, see internal/core/storage) is an opaque blob
-- store addressed by key; this table holds the metadata that makes
-- those blobs findable and auditable: the owning domain and entity,
-- the original file name (never stored in the key), content type,
-- size, ETag and the user who uploaded the file.
--
-- The domain column is an open namespace (patient_document, avatar,
-- ...) so new attachment kinds do not require schema changes; the key
-- layout is owned by the application.
CREATE TABLE storage_objects (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    domain TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL CHECK (size >= 0),
    etag TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_storage_objects_resource ON storage_objects (
    domain, resource_id, created_at DESC
);

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX idx_storage_objects_resource;
DROP TABLE storage_objects;
