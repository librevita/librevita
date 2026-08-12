-- Queries for patient identifiers (FHIR-style system + value, encrypted
-- at field level). Exact lookups match on blind_index only: it is a
-- keyed BLAKE2b-256 digest of system || '\x00' || normalized_value,
-- derived by the application. Never use LIKE/instr() on blind_index --
-- the digest is not prefixable, and pattern matches would leak
-- correlations between rows.

-- name: CreatePatientIdentifier :one
INSERT INTO patient_identifiers (
    id, patient_id, system, value_ciphertext, nonce, blind_index, created_by
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: FindPatientByBlindIndex :one
SELECT pi.*, p.display_name
FROM patient_identifiers pi
JOIN patients p ON p.id = pi.patient_id
WHERE pi.blind_index = ? AND p.clinic_id = ?
LIMIT 1;

-- name: FindIdentifiersByPatient :many
SELECT *
FROM patient_identifiers
WHERE patient_id = ?
ORDER BY system, created_at;

-- name: DeletePatientIdentifier :exec
DELETE FROM patient_identifiers
WHERE id = ? AND patient_id = ?;

-- name: UpdateIdentifierValue :one
-- Used only by the offline re-key routine; never called from the web
-- path.
UPDATE patient_identifiers
SET
    value_ciphertext = ?,
    nonce = ?,
    blind_index = ?,
    updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ?
RETURNING *;
