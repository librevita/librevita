-- +goose Up
-- +goose NO TRANSACTION
-- Staff profile change requests: a receptionist proposes changes to a
-- physician's record (name, email, specialties) and an administrator
-- approves or rejects them. The proposed fields are stored as JSON in
-- changes: {"name": "...", "email": "...", "specialties": ["id", ...]}.

CREATE TABLE staff_change_requests (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'approved', 'rejected')),
    changes TEXT NOT NULL,
    decision_note TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    decided_at TEXT,
    decided_by TEXT REFERENCES users(id)
);

CREATE INDEX idx_staff_requests_status ON staff_change_requests (
    status, created_at DESC
);
CREATE INDEX idx_staff_requests_user ON staff_change_requests (user_id);

-- +goose Down
-- +goose NO TRANSACTION
DROP TABLE staff_change_requests;
