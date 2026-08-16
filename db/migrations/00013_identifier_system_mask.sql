-- +goose Up
-- +goose NO TRANSACTION
-- Typing mask for the document value field, per identifier system
-- (Alpine mask syntax: 9 digit, A uppercase, a lowercase, * any; the
-- rest are literals). Optional: empty means no mask, the pattern and
-- transform are still the validation authority.

ALTER TABLE identifier_systems ADD COLUMN mask TEXT NOT NULL DEFAULT '';

-- +goose Down
-- +goose NO TRANSACTION
ALTER TABLE identifier_systems DROP COLUMN mask;
