-- +goose Up
ALTER TABLE "patient_identifiers" DROP COLUMN "nonce";

-- +goose Down
ALTER TABLE "patient_identifiers" ADD COLUMN "nonce" bytea NOT NULL DEFAULT '\x';
ALTER TABLE "patient_identifiers" ALTER COLUMN "nonce" DROP DEFAULT;
