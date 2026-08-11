-- +goose Up
-- +goose NO TRANSACTION
-- Canonical application-level checksum (BLAKE2b-256, hex) of the stored
-- blob. Unlike the backend ETag (S3 multipart ETags are not the digest
-- of the whole object), this checksum is computed by the application in
-- both backends, so the value is comparable and can be witnessed in the
-- audit chain at upload time.
ALTER TABLE storage_objects ADD COLUMN checksum TEXT NOT NULL DEFAULT '';

-- +goose Down
-- +goose NO TRANSACTION
ALTER TABLE storage_objects DROP COLUMN checksum;
