-- +goose Up
-- Multi-clinic isolation for PostgreSQL. Existing rows attach to the oldest clinic.

ALTER TABLE "clinics" ADD COLUMN "slug" character varying;
ALTER TABLE "clinics" ADD COLUMN "onboarded_at" timestamptz;
UPDATE "clinics"
SET
  "slug" = CASE
    WHEN "id" = (SELECT "id" FROM "clinics" ORDER BY "created_at" ASC LIMIT 1) THEN 'default'
    ELSE 'c-' || replace("id"::text, '-', '')
  END,
  "onboarded_at" = "created_at"
WHERE "slug" IS NULL;
ALTER TABLE "clinics" ALTER COLUMN "slug" SET NOT NULL;
CREATE UNIQUE INDEX "clinics_slug_key" ON "clinics" ("slug");

ALTER TABLE "roles" ADD COLUMN "clinic_id" uuid;
UPDATE "roles" SET "clinic_id" = (SELECT "id" FROM "clinics" ORDER BY "created_at" ASC LIMIT 1);
ALTER TABLE "roles" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "roles" ADD CONSTRAINT "roles_clinics_roles" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
DROP INDEX IF EXISTS "roles_name_key";
CREATE UNIQUE INDEX "role_clinic_id_name" ON "roles" ("clinic_id", "name");

ALTER TABLE "users" ADD COLUMN "clinic_id" uuid;
UPDATE "users" SET "clinic_id" = (SELECT "id" FROM "clinics" ORDER BY "created_at" ASC LIMIT 1);
ALTER TABLE "users" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "users" ADD CONSTRAINT "users_clinics_users" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
DROP INDEX IF EXISTS "users_email_key";
CREATE UNIQUE INDEX "user_clinic_id_email" ON "users" ("clinic_id", "email");

ALTER TABLE "policies" ADD COLUMN "clinic_id" uuid;
UPDATE "policies" SET "clinic_id" = (SELECT "id" FROM "clinics" ORDER BY "created_at" ASC LIMIT 1);
ALTER TABLE "policies" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "policies" ADD CONSTRAINT "policies_clinics_policies" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
DROP INDEX IF EXISTS "policies_name_key";
CREATE UNIQUE INDEX "accesspolicy_clinic_id_name" ON "policies" ("clinic_id", "name");

ALTER TABLE "patients" ADD COLUMN "user_id" uuid;
ALTER TABLE "patients" ADD CONSTRAINT "patients_users_portal_patient" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
CREATE UNIQUE INDEX "patient_clinic_id_user_id" ON "patients" ("clinic_id", "user_id") WHERE "user_id" IS NOT NULL;

ALTER TABLE "patient_identifiers" ADD COLUMN "clinic_id" uuid;
UPDATE "patient_identifiers" pi
SET "clinic_id" = p."clinic_id"
FROM "patients" p
WHERE p."id" = pi."patient_id";
ALTER TABLE "patient_identifiers" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "patient_identifiers" ADD CONSTRAINT "patient_identifiers_clinics_patient_identifiers" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
DROP INDEX IF EXISTS "patient_identifiers_blind_index_key";
CREATE UNIQUE INDEX "patientidentifier_clinic_id_blind_index" ON "patient_identifiers" ("clinic_id", "blind_index");

ALTER TABLE "staff_change_requests" ADD COLUMN "clinic_id" uuid;
UPDATE "staff_change_requests" scr
SET "clinic_id" = u."clinic_id"
FROM "users" u
WHERE u."id" = scr."user_id";
ALTER TABLE "staff_change_requests" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "staff_change_requests" ADD CONSTRAINT "staff_change_requests_clinics_staff_requests" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
CREATE INDEX "staffchangerequest_clinic_id" ON "staff_change_requests" ("clinic_id");

ALTER TABLE "storage_objects" ADD COLUMN "clinic_id" uuid;
UPDATE "storage_objects" so
SET "clinic_id" = u."clinic_id"
FROM "users" u
WHERE u."id" = so."created_by";
ALTER TABLE "storage_objects" ALTER COLUMN "clinic_id" SET NOT NULL;
ALTER TABLE "storage_objects" ADD CONSTRAINT "storage_objects_clinics_storage_objects" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
CREATE INDEX "storageobject_clinic_id" ON "storage_objects" ("clinic_id");

ALTER TABLE "audit_log" ADD COLUMN "clinic_id" uuid;
ALTER TABLE "audit_log" ADD CONSTRAINT "audit_log_clinics_audit_logs" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
CREATE INDEX "auditlog_clinic_id_id" ON "audit_log" ("clinic_id", "id");

CREATE TABLE "platform_users" (
  "id" uuid NOT NULL,
  "email" character varying NOT NULL,
  "password_hash" character varying NOT NULL,
  "display_name" character varying NOT NULL,
  "active" boolean NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "platform_users_email_key" ON "platform_users" ("email");

CREATE TABLE "platform_sessions" (
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "platform_user_id" uuid NOT NULL,
  PRIMARY KEY ("token_hash"),
  CONSTRAINT "platform_sessions_platform_users_sessions" FOREIGN KEY ("platform_user_id") REFERENCES "platform_users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
CREATE INDEX "platformsession_platform_user_id" ON "platform_sessions" ("platform_user_id");
CREATE INDEX "platformsession_expires_at" ON "platform_sessions" ("expires_at");

CREATE TABLE "clinic_identifier_systems" (
  "id" uuid NOT NULL,
  "clinic_id" uuid NOT NULL,
  "identifier_system_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "clinic_identifier_systems_clinics_identifier_systems" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "clinic_identifier_systems_identifier_systems_clinic_opt_ins" FOREIGN KEY ("identifier_system_id") REFERENCES "identifier_systems" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
CREATE UNIQUE INDEX "clinicidentifiersystem_clinic_id_identifier_system_id" ON "clinic_identifier_systems" ("clinic_id", "identifier_system_id");
INSERT INTO "clinic_identifier_systems" ("id", "clinic_id", "identifier_system_id")
SELECT gen_random_uuid(), c."id", s."id"
FROM "clinics" c
CROSS JOIN "identifier_systems" s
WHERE s."active" = true;

-- +goose Down
SELECT pg_catalog.set_config('librevita.fail', 'multi_clinic down is not supported', false);
SELECT 1/0;
