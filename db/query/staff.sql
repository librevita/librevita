-- Queries for the staff domain.
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
