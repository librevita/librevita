-- Queries for the patient domain. Timestamps use the database default so
-- the format stays consistent with the migrations.

-- name: CreatePatient :one
INSERT INTO patients (
    id, clinic_id, display_name, birth_date, sex, document,
    phone, email, street, city, state, postal_code, notes, created_by
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPatientWithCreator :one
SELECT p.*, u.email AS created_by_email
FROM patients p
LEFT JOIN users u ON u.id = p.created_by
WHERE p.id = ? AND p.clinic_id = ?
LIMIT 1;

-- name: GetPatientByID :one
SELECT *
FROM patients
WHERE id = ? AND clinic_id = ?
LIMIT 1;

-- name: UpdatePatient :one
UPDATE patients
SET
    display_name = ?,
    birth_date = ?,
    sex = ?,
    document = ?,
    phone = ?,
    email = ?,
    street = ?,
    city = ?,
    state = ?,
    postal_code = ?,
    notes = ?,
    updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND clinic_id = ?
RETURNING *;

-- name: UpdatePatientStatus :exec
UPDATE patients
SET status = ?, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND clinic_id = ?;

-- name: CountPatients :one
SELECT count(*)
FROM patients
WHERE clinic_id = ?;

-- name: PatientDocumentExists :one
SELECT exists(
    SELECT 1
    FROM patients
    WHERE clinic_id = ? AND document = ? AND id <> ?
);

-- name: ListPatientsPage :many
-- instr() searches the term literally (no LIKE wildcards), matching
-- word prefixes: the term is anchored at a word boundary with a space.
SELECT *
FROM patients
WHERE
    clinic_id = ?
    AND (@status_empty = '' OR status = @status_filter)
    AND (
        @query_empty = ''
        OR instr(
            lower(
                ' '
                || display_name
                || ' '
                || coalesce(document, '')
                || ' '
                || coalesce(email, '')
            ),
            lower(' ' || @pattern)
        ) > 0
    )
ORDER BY display_name COLLATE NOCASE
LIMIT @limit OFFSET @offset;

-- name: CountPatientsMatching :one
SELECT COUNT(*)
FROM patients
WHERE clinic_id = ?
AND (@status_empty = '' OR status = @status_filter)
AND (@query_empty = '' OR instr(
lower(' ' || display_name || ' ' || COALESCE(document,
'') || ' ' || COALESCE(email,
'')),
lower(' ' || @pattern)
) > 0);
