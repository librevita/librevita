-- +goose Up
-- +goose NO TRANSACTION
-- Typing masks for the seeded document systems. NIF (Portugal) and
-- the passport are left without a mask: neither has a standard
-- punctuation, and the passport length varies.

UPDATE identifier_systems SET mask = '999.999.999-99'
WHERE system = 'urn:librevita:id:br:cpf'
;
UPDATE identifier_systems SET mask = '999 9999 9999 9999'
WHERE system = 'urn:librevita:id:br:sus'
;

-- +goose Down
-- +goose NO TRANSACTION
UPDATE identifier_systems SET mask = '' WHERE system IN (
    'urn:librevita:id:br:cpf',
    'urn:librevita:id:br:sus'
);
