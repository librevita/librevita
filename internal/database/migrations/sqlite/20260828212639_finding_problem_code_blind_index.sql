-- +goose Up
-- disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;

-- create "new_findings" table
CREATE TABLE `new_findings` (
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
  `code_blind_index` text NULL,
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

-- copy rows from old table "findings" to new temporary table "new_findings"
INSERT INTO `new_findings` (`id`, `status`, `value_kind`, `effective_at`, `code_system`, `code`, `display`, `value_number`, `value_unit`, `value_ucum`, `value_text`, `value_bool`, `value_coded_system`, `value_coded_code`, `value_coded_display`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id`) SELECT `id`, `status`, `value_kind`, `effective_at`, `code_system`, `code`, `display`, `value_number`, `value_unit`, `value_ucum`, `value_text`, `value_bool`, `value_coded_system`, `value_coded_code`, `value_coded_display`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id` FROM `findings`;

-- drop "findings" table after copying rows
DROP TABLE `findings`;

-- rename temporary table "new_findings" to "findings"
ALTER TABLE `new_findings` RENAME TO `findings`;

-- create index "finding_clinic_id_episode_id" to table: "findings"
CREATE INDEX `finding_clinic_id_episode_id` ON `findings` (`clinic_id`, `episode_id`);

-- create index "finding_patient_id_created_at" to table: "findings"
CREATE INDEX `finding_patient_id_created_at` ON `findings` (`patient_id`, `created_at`);

-- create index "finding_episode_id" to table: "findings"
CREATE INDEX `finding_episode_id` ON `findings` (`episode_id`);

-- create index "finding_clinic_id_code_blind_index" to table: "findings"
CREATE INDEX `finding_clinic_id_code_blind_index` ON `findings` (`clinic_id`, `code_blind_index`);

-- create "new_problems" table
CREATE TABLE `new_problems` (
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
  `code_blind_index` text NULL,
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

-- copy rows from old table "problems" to new temporary table "new_problems"
INSERT INTO `new_problems` (`id`, `clinical_status`, `verification_status`, `category`, `rank`, `code_system`, `code`, `display`, `text`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id`) SELECT `id`, `clinical_status`, `verification_status`, `category`, `rank`, `code_system`, `code`, `display`, `text`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id` FROM `problems`;

-- drop "problems" table after copying rows
DROP TABLE `problems`;

-- rename temporary table "new_problems" to "problems"
ALTER TABLE `new_problems` RENAME TO `problems`;

-- create index "problem_clinic_id_episode_id" to table: "problems"
CREATE INDEX `problem_clinic_id_episode_id` ON `problems` (`clinic_id`, `episode_id`);

-- create index "problem_patient_id_created_at" to table: "problems"
CREATE INDEX `problem_patient_id_created_at` ON `problems` (`patient_id`, `created_at`);

-- create index "problem_episode_id" to table: "problems"
CREATE INDEX `problem_episode_id` ON `problems` (`episode_id`);

-- create index "problem_clinic_id_code_blind_index" to table: "problems"
CREATE INDEX `problem_clinic_id_code_blind_index` ON `problems` (`clinic_id`, `code_blind_index`);

-- enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;

-- +goose Down
PRAGMA foreign_keys = off;

DROP INDEX IF EXISTS `problem_clinic_id_code_blind_index`;
DROP INDEX IF EXISTS `problem_episode_id`;
DROP INDEX IF EXISTS `problem_patient_id_created_at`;
DROP INDEX IF EXISTS `problem_clinic_id_episode_id`;

CREATE TABLE `old_problems` (
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

INSERT INTO `old_problems` (`id`, `clinical_status`, `verification_status`, `category`, `rank`, `code_system`, `code`, `display`, `text`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id`)
SELECT `id`, `clinical_status`, `verification_status`, `category`, `rank`, `code_system`, `code`, `display`, `text`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id` FROM `problems`;

DROP TABLE `problems`;
ALTER TABLE `old_problems` RENAME TO `problems`;

CREATE INDEX `problem_clinic_id_episode_id` ON `problems` (`clinic_id`, `episode_id`);
CREATE INDEX `problem_patient_id_created_at` ON `problems` (`patient_id`, `created_at`);
CREATE INDEX `problem_episode_id` ON `problems` (`episode_id`);

DROP INDEX IF EXISTS `finding_clinic_id_code_blind_index`;
DROP INDEX IF EXISTS `finding_episode_id`;
DROP INDEX IF EXISTS `finding_patient_id_created_at`;
DROP INDEX IF EXISTS `finding_clinic_id_episode_id`;

CREATE TABLE `old_findings` (
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

INSERT INTO `old_findings` (`id`, `status`, `value_kind`, `effective_at`, `code_system`, `code`, `display`, `value_number`, `value_unit`, `value_ucum`, `value_text`, `value_bool`, `value_coded_system`, `value_coded_code`, `value_coded_display`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id`)
SELECT `id`, `status`, `value_kind`, `effective_at`, `code_system`, `code`, `display`, `value_number`, `value_unit`, `value_ucum`, `value_text`, `value_bool`, `value_coded_system`, `value_coded_code`, `value_coded_display`, `created_at`, `updated_at`, `clinic_id`, `episode_id`, `patient_id` FROM `findings`;

DROP TABLE `findings`;
ALTER TABLE `old_findings` RENAME TO `findings`;

CREATE INDEX `finding_clinic_id_episode_id` ON `findings` (`clinic_id`, `episode_id`);
CREATE INDEX `finding_patient_id_created_at` ON `findings` (`patient_id`, `created_at`);
CREATE INDEX `finding_episode_id` ON `findings` (`episode_id`);

PRAGMA foreign_keys = on;
