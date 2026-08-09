-- Queries for the session revocation index. The PASETO token itself carries
-- the session payload; this table exists only to revoke sessions on logout
-- and to reject deactivated accounts.

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES (?, ?, ?);

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= ?;

-- name: GetSessionUser :one
SELECT u.id, u.email, u.display_name, r.name AS role_name
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN roles r ON r.id = u.role_id
WHERE s.token_hash = ? AND u.active = 1
LIMIT 1;
