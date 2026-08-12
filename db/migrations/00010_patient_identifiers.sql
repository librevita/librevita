-- +goose Up
-- +goose NO TRANSACTION
-- Patient identification documents, modeled after the HL7 FHIR
-- Identifier (system + value): a patient may hold several documents
-- (CPF, RG, SUS card, passport, NIF, ...) from any jurisdiction, so no
-- fixed column such as tax_id can exist in a generic, global product.
--
-- The plaintext value lives only in the application. value_ciphertext
-- is the XChaCha20-Poly1305 ciphertext of the normalized value, with
-- its random 24-byte nonce in its own column; blind_index is a keyed
-- BLAKE2b-256 digest of system || '\x00' || normalized_value,
-- hex-encoded, derived by the application from a purpose-specific HKDF
-- key. Exact lookups (a registrar typing a CPF) use equality on
-- blind_index only; the ciphertext is never indexed or compared.
--
-- UNIQUE(blind_index) enforces the real-world invariant that a value
-- is unique within its system (a CPF belongs to exactly one patient,
-- anywhere in the deployment). Multiple values per (patient_id, system)
-- remain allowed (e.g. two passports), so there is deliberately no
-- UNIQUE(patient_id, system).

-- identifier_systems defines which document kinds exist: the system
-- URN, a human label, and the validation rules. Like roles, systems
-- are rows, not code: an administrator registers a new document type
-- (a Paraguayan cédula, an Argentine DNI, ...) without a deployment.
--
--   pattern        regex of the document shape, applied to the
--                  transformed value (anchored: ^(?:pattern)$)
--   transform      how raw input is canonicalized before matching:
--                  none (trim + collapse spaces), digits (only
--                  digits), upper, lower
--   check_*        optional check digit: mod11_desc (descending
--                  weights, e.g. CPF/NIF) or mod11_cyclic (weights
--                  2..9 right-to-left, e.g. SUS); none disables the
--                  check. check_base_len counts the digits without
--                  the check digit(s), check_dv_count how many (1 or
--                  2; mod11_cyclic requires 1), check_start_weight
--                  the first weight for mod11_desc.
-- The seeds cover the most common deployments (Brazil, Portugal, and
-- ICAO passports); they can be edited or deactivated like any other
-- system. "urn:librevita:id:raw" is reserved as the built-in fallback
-- that accepts any value.
CREATE TABLE identifier_systems (
    id TEXT PRIMARY KEY,
    system TEXT NOT NULL UNIQUE CHECK (length(system) BETWEEN 3 AND 64),
    display_name TEXT NOT NULL,
    pattern TEXT NOT NULL,
    transform TEXT NOT NULL DEFAULT 'none'
    CHECK (transform IN ('none', 'digits', 'upper', 'lower')),
    check_algorithm TEXT NOT NULL DEFAULT 'none'
    CHECK (check_algorithm IN ('none', 'mod11_desc', 'mod11_cyclic')),
    check_base_len INTEGER NOT NULL DEFAULT 0 CHECK (check_base_len >= 0),
    check_dv_count INTEGER NOT NULL DEFAULT 1 CHECK (check_dv_count IN (1, 2)),
    check_start_weight INTEGER NOT NULL DEFAULT 10 CHECK (
        check_start_weight >= 2
    ),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

INSERT INTO identifier_systems (
    id, system, display_name, pattern, transform,
    check_algorithm, check_base_len, check_dv_count, check_start_weight
) VALUES
(
    '00000000-0000-7000-8000-000000000011',
    'urn:librevita:id:br:cpf',
    'CPF (Brasil)',
    '[0-9]{11}',
    'digits',
    'mod11_desc', 9, 2, 10
),
(
    '00000000-0000-7000-8000-000000000012',
    'urn:librevita:id:br:sus',
    'Cartão SUS (Brasil)',
    '[0-9]{15}',
    'digits',
    'mod11_cyclic', 14, 1, 10
),
(
    '00000000-0000-7000-8000-000000000013',
    'urn:librevita:id:pt:nif',
    'NIF (Portugal)',
    '[0-9]{9}',
    'digits',
    'mod11_desc', 8, 1, 9
),
(
    '00000000-0000-7000-8000-000000000014',
    'urn:librevita:id:passport',
    'Passaporte',
    '[A-Z]{1,2}[0-9]{6,9}',
    'upper',
    'none', 0, 1, 10
);

CREATE TABLE patient_identifiers (
    id TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    system TEXT NOT NULL REFERENCES identifier_systems(system),
    value_ciphertext BLOB NOT NULL CHECK (length(value_ciphertext) > 0),
    nonce BLOB NOT NULL CHECK (length(nonce) = 24),
    blind_index TEXT NOT NULL UNIQUE CHECK (length(blind_index) = 64),
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX idx_patient_identifiers_patient ON patient_identifiers (
    patient_id
);
CREATE INDEX idx_patient_identifiers_system ON patient_identifiers (system);

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX idx_patient_identifiers_system;
DROP INDEX idx_patient_identifiers_patient;
DROP TABLE patient_identifiers;
DROP TABLE identifier_systems;
