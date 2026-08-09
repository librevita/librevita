-- +goose Up
-- +goose NO TRANSACTION
-- Relational roles: the account role is a row in roles instead of a
-- fixed TEXT CHECK enum, so the administrator can create new roles
-- (psychologist, manager, ...) at runtime. The four original roles are
-- seeded as system roles: they cannot be renamed or deleted, because the
-- CEL policies reference them by name.

CREATE TABLE roles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE COLLATE nocase,
    system      INTEGER NOT NULL DEFAULT 0 CHECK (system IN (0, 1)),
    is_clinical INTEGER NOT NULL DEFAULT 0 CHECK (is_clinical IN (0, 1)),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO roles (id, name, system, is_clinical) VALUES
('00000000-0000-7000-8000-000000000001', 'admin', 1, 0),
('00000000-0000-7000-8000-000000000002', 'physician', 1, 1),
('00000000-0000-7000-8000-000000000003', 'receptionist', 1, 0),
('00000000-0000-7000-8000-000000000004', 'patient', 1, 0);

ALTER TABLE users ADD COLUMN role_id TEXT NOT NULL DEFAULT '00000000-0000-7000-8000-000000000004' REFERENCES roles(
    id
);

UPDATE users SET role_id = '00000000-0000-7000-8000-000000000001'
WHERE role = 'admin'
;
UPDATE users SET role_id = '00000000-0000-7000-8000-000000000002'
WHERE role = 'physician'
;
UPDATE users SET role_id = '00000000-0000-7000-8000-000000000003'
WHERE role = 'receptionist'
;
UPDATE users SET role_id = '00000000-0000-7000-8000-000000000004'
WHERE role = 'patient'
;

ALTER TABLE users DROP COLUMN role;

CREATE INDEX idx_users_role ON users (role_id);

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX idx_users_role;
ALTER TABLE roles DROP COLUMN is_clinical;
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'patient'
CHECK (role IN ('admin', 'physician', 'receptionist', 'patient'));
UPDATE users SET role = 'admin'
WHERE role_id = '00000000-0000-7000-8000-000000000001'
;
UPDATE users SET role = 'physician'
WHERE role_id = '00000000-0000-7000-8000-000000000002'
;
UPDATE users SET role = 'receptionist'
WHERE role_id = '00000000-0000-7000-8000-000000000003'
;
UPDATE users SET role = 'patient'
WHERE role_id = '00000000-0000-7000-8000-000000000004'
;
ALTER TABLE users DROP COLUMN role_id;
DROP TABLE roles;
