-- +goose Up
-- Exact-match blind indexes for clinical codes (Patient DEK, clinic-scoped).

ALTER TABLE "findings" ADD COLUMN "code_blind_index" character varying NULL;
CREATE INDEX "finding_clinic_id_code_blind_index" ON "findings" ("clinic_id", "code_blind_index");

ALTER TABLE "problems" ADD COLUMN "code_blind_index" character varying NULL;
CREATE INDEX "problem_clinic_id_code_blind_index" ON "problems" ("clinic_id", "code_blind_index");

-- +goose Down
DROP INDEX IF EXISTS "problem_clinic_id_code_blind_index";
ALTER TABLE "problems" DROP COLUMN "code_blind_index";

DROP INDEX IF EXISTS "finding_clinic_id_code_blind_index";
ALTER TABLE "findings" DROP COLUMN "code_blind_index";
