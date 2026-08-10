-- Queries for the meta domain.
-- name: GetMetaValue :one
SELECT value
FROM meta
WHERE key = ?
LIMIT 1;
-- name: SetMeta :exec
INSERT INTO meta (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
