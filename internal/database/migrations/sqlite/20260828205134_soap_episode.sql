-- +goose Up
-- disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;

-- create "new_episodes" table
CREATE TABLE `new_episodes` (
  `id` uuid NOT NULL,
  `episode_type` text NOT NULL DEFAULT ('consultation'),
  `status` text NOT NULL DEFAULT ('draft'),
  `class` text NOT NULL DEFAULT ('ambulatory'),
  `occurred_at` datetime NOT NULL,
  `ended_at` datetime NULL,
  `subjective` blob NULL,
  `objective` blob NULL,
  `assessment` blob NULL,
  `plan` blob NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `appointment_id` uuid NULL,
  `clinic_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `episodes_appointments_episodes` FOREIGN KEY (`appointment_id`) REFERENCES `appointments` (`id`) ON DELETE SET NULL,
  CONSTRAINT `episodes_clinics_episodes` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_patients_episodes` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_users_episodes` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_class_check` CHECK (class IN ('ambulatory', 'emergency', 'inpatient', 'virtual')),
  CONSTRAINT `episodes_episode_type_check` CHECK (episode_type IN ('consultation', 'anamnesis', 'evolution', 'prescription', 'exam_request', 'diagnostic')),
  CONSTRAINT `episodes_status_check` CHECK (status IN ('draft', 'finalized', 'archived'))
);

-- copy rows from old table "episodes" to new temporary table "new_episodes"
INSERT INTO `new_episodes` (`id`, `episode_type`, `status`, `class`, `occurred_at`, `ended_at`, `subjective`, `objective`, `assessment`, `plan`, `created_at`, `updated_at`, `appointment_id`, `clinic_id`, `patient_id`, `user_id`)
SELECT `id`, `episode_type`, `status`, 'ambulatory', `created_at`, NULL, `notes`, NULL, `diagnostic`, `prescription`, `created_at`, `updated_at`, `appointment_id`, `clinic_id`, `patient_id`, `user_id` FROM `episodes`;

-- drop "episodes" table after copying rows
DROP TABLE `episodes`;

-- rename temporary table "new_episodes" to "episodes"
ALTER TABLE `new_episodes` RENAME TO `episodes`;

-- create index "episode_clinic_id_created_at" to table: "episodes"
CREATE INDEX `episode_clinic_id_created_at` ON `episodes` (`clinic_id`, `created_at`);

-- create index "episode_patient_id_created_at" to table: "episodes"
CREATE INDEX `episode_patient_id_created_at` ON `episodes` (`patient_id`, `created_at`);

-- create index "episode_user_id_created_at" to table: "episodes"
CREATE INDEX `episode_user_id_created_at` ON `episodes` (`user_id`, `created_at`);

-- create index "episode_episode_type" to table: "episodes"
CREATE INDEX `episode_episode_type` ON `episodes` (`episode_type`);

-- create index "episode_status" to table: "episodes"
CREATE INDEX `episode_status` ON `episodes` (`status`);

-- create "findings" table
CREATE TABLE `findings` (
  `id` uuid NOT NULL,
  `status` text NOT NULL DEFAULT ('recorded'),
  `value_kind` text NOT NULL,
  `effective_at` datetime NOT NULL,
  `code_system` blob NULL,
  `code` blob NULL,
  `display` blob NULL,
  `value_number` blob NULL,
  `value_unit` blob NULL,
  `value_ucum` blob NULL,
  `value_text` blob NULL,
  `value_bool` blob NULL,
  `value_coded_system` blob NULL,
  `value_coded_code` blob NULL,
  `value_coded_display` blob NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `episode_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `findings_clinics_findings` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `findings_episodes_findings` FOREIGN KEY (`episode_id`) REFERENCES `episodes` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `findings_patients_findings` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `findings_status_check` CHECK (status IN ('recorded', 'provisional', 'cancelled')),
  CONSTRAINT `findings_value_kind_check` CHECK (value_kind IN ('quantity', 'string', 'boolean', 'coded'))
);

-- create index "finding_clinic_id_episode_id" to table: "findings"
CREATE INDEX `finding_clinic_id_episode_id` ON `findings` (`clinic_id`, `episode_id`);

-- create index "finding_patient_id_created_at" to table: "findings"
CREATE INDEX `finding_patient_id_created_at` ON `findings` (`patient_id`, `created_at`);

-- create index "finding_episode_id" to table: "findings"
CREATE INDEX `finding_episode_id` ON `findings` (`episode_id`);

-- create "plan_items" table
CREATE TABLE `plan_items` (
  `id` uuid NOT NULL,
  `kind` text NOT NULL DEFAULT ('instruction'),
  `status` text NOT NULL DEFAULT ('active'),
  `scheduled_at` datetime NULL,
  `code_system` blob NULL,
  `code` blob NULL,
  `display` blob NULL,
  `description` blob NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `episode_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `plan_items_clinics_plan_items` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `plan_items_episodes_plan_items` FOREIGN KEY (`episode_id`) REFERENCES `episodes` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `plan_items_patients_plan_items` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `plan_items_kind_check` CHECK (kind IN ('medication', 'procedure', 'exam', 'appointment', 'instruction')),
  CONSTRAINT `plan_items_status_check` CHECK (status IN ('draft', 'active', 'completed', 'cancelled'))
);

-- create index "planitem_clinic_id_episode_id" to table: "plan_items"
CREATE INDEX `planitem_clinic_id_episode_id` ON `plan_items` (`clinic_id`, `episode_id`);

-- create index "planitem_patient_id_created_at" to table: "plan_items"
CREATE INDEX `planitem_patient_id_created_at` ON `plan_items` (`patient_id`, `created_at`);

-- create index "planitem_episode_id" to table: "plan_items"
CREATE INDEX `planitem_episode_id` ON `plan_items` (`episode_id`);

-- create "problems" table
CREATE TABLE `problems` (
  `id` uuid NOT NULL,
  `clinical_status` text NOT NULL DEFAULT ('active'),
  `verification_status` text NOT NULL DEFAULT ('confirmed'),
  `category` text NOT NULL DEFAULT ('encounter'),
  `rank` integer NOT NULL DEFAULT (1),
  `code_system` blob NULL,
  `code` blob NULL,
  `display` blob NULL,
  `text` blob NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `episode_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `problems_clinics_problems` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `problems_episodes_problems` FOREIGN KEY (`episode_id`) REFERENCES `episodes` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `problems_patients_problems` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `problems_category_check` CHECK (category IN ('encounter', 'list')),
  CONSTRAINT `problems_clinical_status_check` CHECK (clinical_status IN ('active', 'inactive', 'resolved')),
  CONSTRAINT `problems_verification_status_check` CHECK (verification_status IN ('confirmed', 'suspected', 'refuted', 'error'))
);

-- create index "problem_clinic_id_episode_id" to table: "problems"
CREATE INDEX `problem_clinic_id_episode_id` ON `problems` (`clinic_id`, `episode_id`);

-- create index "problem_patient_id_created_at" to table: "problems"
CREATE INDEX `problem_patient_id_created_at` ON `problems` (`patient_id`, `created_at`);

-- create index "problem_episode_id" to table: "problems"
CREATE INDEX `problem_episode_id` ON `problems` (`episode_id`);

-- enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;

-- +goose Down
PRAGMA foreign_keys = off;

DROP INDEX IF EXISTS `problem_episode_id`;
DROP INDEX IF EXISTS `problem_patient_id_created_at`;
DROP INDEX IF EXISTS `problem_clinic_id_episode_id`;
DROP TABLE IF EXISTS `problems`;

DROP INDEX IF EXISTS `planitem_episode_id`;
DROP INDEX IF EXISTS `planitem_patient_id_created_at`;
DROP INDEX IF EXISTS `planitem_clinic_id_episode_id`;
DROP TABLE IF EXISTS `plan_items`;

DROP INDEX IF EXISTS `finding_episode_id`;
DROP INDEX IF EXISTS `finding_patient_id_created_at`;
DROP INDEX IF EXISTS `finding_clinic_id_episode_id`;
DROP TABLE IF EXISTS `findings`;

CREATE TABLE `old_episodes` (
  `id` uuid NOT NULL,
  `episode_type` text NOT NULL DEFAULT ('consultation'),
  `status` text NOT NULL DEFAULT ('draft'),
  `notes` blob NULL,
  `prescription` blob NULL,
  `diagnostic` blob NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `appointment_id` uuid NULL,
  `clinic_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `episodes_appointments_episodes` FOREIGN KEY (`appointment_id`) REFERENCES `appointments` (`id`) ON DELETE SET NULL,
  CONSTRAINT `episodes_clinics_episodes` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_patients_episodes` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_users_episodes` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `episodes_episode_type_check` CHECK (episode_type IN ('consultation', 'anamnesis', 'evolution', 'prescription', 'exam_request', 'diagnostic')),
  CONSTRAINT `episodes_status_check` CHECK (status IN ('draft', 'finalized', 'archived'))
);

INSERT INTO `old_episodes` (`id`, `episode_type`, `status`, `notes`, `prescription`, `diagnostic`, `created_at`, `updated_at`, `appointment_id`, `clinic_id`, `patient_id`, `user_id`)
SELECT `id`, `episode_type`, `status`, `subjective`, `plan`, `assessment`, `created_at`, `updated_at`, `appointment_id`, `clinic_id`, `patient_id`, `user_id` FROM `episodes`;

DROP TABLE `episodes`;
ALTER TABLE `old_episodes` RENAME TO `episodes`;

CREATE INDEX `episode_clinic_id_created_at` ON `episodes` (`clinic_id`, `created_at`);
CREATE INDEX `episode_patient_id_created_at` ON `episodes` (`patient_id`, `created_at`);
CREATE INDEX `episode_user_id_created_at` ON `episodes` (`user_id`, `created_at`);
CREATE INDEX `episode_episode_type` ON `episodes` (`episode_type`);
CREATE INDEX `episode_status` ON `episodes` (`status`);

PRAGMA foreign_keys = on;


