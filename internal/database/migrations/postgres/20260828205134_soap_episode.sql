-- +goose Up
-- SOAP episode narrative plus structured Finding / Problem / PlanItem children.

ALTER TABLE "episodes" ADD COLUMN "class" character varying NOT NULL DEFAULT 'ambulatory';
ALTER TABLE "episodes" ADD COLUMN "occurred_at" timestamptz;
UPDATE "episodes" SET "occurred_at" = "created_at" WHERE "occurred_at" IS NULL;
ALTER TABLE "episodes" ALTER COLUMN "occurred_at" SET NOT NULL;
ALTER TABLE "episodes" ADD COLUMN "ended_at" timestamptz NULL;
ALTER TABLE "episodes" ADD COLUMN "subjective" bytea NULL;
ALTER TABLE "episodes" ADD COLUMN "objective" bytea NULL;
ALTER TABLE "episodes" ADD COLUMN "assessment" bytea NULL;
ALTER TABLE "episodes" ADD COLUMN "plan" bytea NULL;
UPDATE "episodes" SET "subjective" = "notes", "assessment" = "diagnostic", "plan" = "prescription";
ALTER TABLE "episodes" DROP COLUMN "notes";
ALTER TABLE "episodes" DROP COLUMN "prescription";
ALTER TABLE "episodes" DROP COLUMN "diagnostic";
ALTER TABLE "episodes" ADD CONSTRAINT "episodes_class_check" CHECK (("class")::text = ANY ((ARRAY['ambulatory'::character varying, 'emergency'::character varying, 'inpatient'::character varying, 'virtual'::character varying])::text[]));

CREATE TABLE "findings" (
  "id" uuid NOT NULL,
  "status" character varying NOT NULL DEFAULT 'recorded',
  "value_kind" character varying NOT NULL,
  "effective_at" timestamptz NOT NULL,
  "code_system" bytea NULL,
  "code" bytea NULL,
  "display" bytea NULL,
  "value_number" bytea NULL,
  "value_unit" bytea NULL,
  "value_ucum" bytea NULL,
  "value_text" bytea NULL,
  "value_bool" bytea NULL,
  "value_coded_system" bytea NULL,
  "value_coded_code" bytea NULL,
  "value_coded_display" bytea NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "clinic_id" uuid NOT NULL,
  "episode_id" uuid NOT NULL,
  "patient_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "findings_clinics_findings" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "findings_episodes_findings" FOREIGN KEY ("episode_id") REFERENCES "episodes" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "findings_patients_findings" FOREIGN KEY ("patient_id") REFERENCES "patients" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "findings_status_check" CHECK (("status")::text = ANY ((ARRAY['recorded'::character varying, 'provisional'::character varying, 'cancelled'::character varying])::text[])),
  CONSTRAINT "findings_value_kind_check" CHECK (("value_kind")::text = ANY ((ARRAY['quantity'::character varying, 'string'::character varying, 'boolean'::character varying, 'coded'::character varying])::text[]))
);
CREATE INDEX "finding_clinic_id_episode_id" ON "findings" ("clinic_id", "episode_id");
CREATE INDEX "finding_patient_id_created_at" ON "findings" ("patient_id", "created_at");
CREATE INDEX "finding_episode_id" ON "findings" ("episode_id");

CREATE TABLE "plan_items" (
  "id" uuid NOT NULL,
  "kind" character varying NOT NULL DEFAULT 'instruction',
  "status" character varying NOT NULL DEFAULT 'active',
  "scheduled_at" timestamptz NULL,
  "code_system" bytea NULL,
  "code" bytea NULL,
  "display" bytea NULL,
  "description" bytea NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "clinic_id" uuid NOT NULL,
  "episode_id" uuid NOT NULL,
  "patient_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "plan_items_clinics_plan_items" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "plan_items_episodes_plan_items" FOREIGN KEY ("episode_id") REFERENCES "episodes" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "plan_items_patients_plan_items" FOREIGN KEY ("patient_id") REFERENCES "patients" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "plan_items_kind_check" CHECK (("kind")::text = ANY ((ARRAY['medication'::character varying, 'procedure'::character varying, 'exam'::character varying, 'appointment'::character varying, 'instruction'::character varying])::text[])),
  CONSTRAINT "plan_items_status_check" CHECK (("status")::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'completed'::character varying, 'cancelled'::character varying])::text[]))
);
CREATE INDEX "planitem_clinic_id_episode_id" ON "plan_items" ("clinic_id", "episode_id");
CREATE INDEX "planitem_patient_id_created_at" ON "plan_items" ("patient_id", "created_at");
CREATE INDEX "planitem_episode_id" ON "plan_items" ("episode_id");

CREATE TABLE "problems" (
  "id" uuid NOT NULL,
  "clinical_status" character varying NOT NULL DEFAULT 'active',
  "verification_status" character varying NOT NULL DEFAULT 'confirmed',
  "category" character varying NOT NULL DEFAULT 'encounter',
  "rank" bigint NOT NULL DEFAULT 1,
  "code_system" bytea NULL,
  "code" bytea NULL,
  "display" bytea NULL,
  "text" bytea NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "clinic_id" uuid NOT NULL,
  "episode_id" uuid NOT NULL,
  "patient_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "problems_clinics_problems" FOREIGN KEY ("clinic_id") REFERENCES "clinics" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "problems_episodes_problems" FOREIGN KEY ("episode_id") REFERENCES "episodes" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "problems_patients_problems" FOREIGN KEY ("patient_id") REFERENCES "patients" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "problems_category_check" CHECK (("category")::text = ANY ((ARRAY['encounter'::character varying, 'list'::character varying])::text[])),
  CONSTRAINT "problems_clinical_status_check" CHECK (("clinical_status")::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying, 'resolved'::character varying])::text[])),
  CONSTRAINT "problems_verification_status_check" CHECK (("verification_status")::text = ANY ((ARRAY['confirmed'::character varying, 'suspected'::character varying, 'refuted'::character varying, 'error'::character varying])::text[]))
);
CREATE INDEX "problem_clinic_id_episode_id" ON "problems" ("clinic_id", "episode_id");
CREATE INDEX "problem_patient_id_created_at" ON "problems" ("patient_id", "created_at");
CREATE INDEX "problem_episode_id" ON "problems" ("episode_id");

-- +goose Down
DROP INDEX IF EXISTS "problem_episode_id";
DROP INDEX IF EXISTS "problem_patient_id_created_at";
DROP INDEX IF EXISTS "problem_clinic_id_episode_id";
DROP TABLE IF EXISTS "problems";

DROP INDEX IF EXISTS "planitem_episode_id";
DROP INDEX IF EXISTS "planitem_patient_id_created_at";
DROP INDEX IF EXISTS "planitem_clinic_id_episode_id";
DROP TABLE IF EXISTS "plan_items";

DROP INDEX IF EXISTS "finding_episode_id";
DROP INDEX IF EXISTS "finding_patient_id_created_at";
DROP INDEX IF EXISTS "finding_clinic_id_episode_id";
DROP TABLE IF EXISTS "findings";

ALTER TABLE "episodes" DROP CONSTRAINT IF EXISTS "episodes_class_check";
ALTER TABLE "episodes" ADD COLUMN "notes" bytea NULL;
ALTER TABLE "episodes" ADD COLUMN "prescription" bytea NULL;
ALTER TABLE "episodes" ADD COLUMN "diagnostic" bytea NULL;
UPDATE "episodes" SET "notes" = "subjective", "diagnostic" = "assessment", "prescription" = "plan";
ALTER TABLE "episodes" DROP COLUMN "class";
ALTER TABLE "episodes" DROP COLUMN "occurred_at";
ALTER TABLE "episodes" DROP COLUMN "ended_at";
ALTER TABLE "episodes" DROP COLUMN "subjective";
ALTER TABLE "episodes" DROP COLUMN "objective";
ALTER TABLE "episodes" DROP COLUMN "assessment";
ALTER TABLE "episodes" DROP COLUMN "plan";
