-- +goose Up
-- Rename slug column to domain in clinics table
ALTER TABLE "clinics" RENAME COLUMN "slug" TO "domain";
DROP INDEX IF EXISTS "clinics_slug_key";
CREATE UNIQUE INDEX "clinics_domain_key" ON "clinics" ("domain");

-- +goose Down
DROP INDEX IF EXISTS "clinics_domain_key";
ALTER TABLE "clinics" RENAME COLUMN "domain" TO "slug";
CREATE UNIQUE INDEX "clinics_slug_key" ON "clinics" ("slug");
