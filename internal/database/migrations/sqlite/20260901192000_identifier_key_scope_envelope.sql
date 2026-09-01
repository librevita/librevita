-- +goose Up
PRAGMA foreign_keys = off;

CREATE TABLE `new_patient_identifiers` (
  `id` uuid NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `system` text NOT NULL,
  `value_ciphertext` blob NOT NULL,
  `blind_index` text NOT NULL,
  `created_by` uuid NULL,
  `clinic_id` uuid NOT NULL,
  `identifier_system_identifiers` uuid NULL,
  `patient_id` uuid NOT NULL,
  `patient_identifier_identifier_system` uuid NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `patient_identifiers_clinics_patient_identifiers` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patient_identifiers_identifier_systems_identifiers` FOREIGN KEY (`identifier_system_identifiers`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL,
  CONSTRAINT `patient_identifiers_patients_identifiers` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patient_identifiers_identifier_systems_identifier_system` FOREIGN KEY (`patient_identifier_identifier_system`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL
);

INSERT INTO `new_patient_identifiers` (`id`, `created_at`, `updated_at`, `system`, `value_ciphertext`, `blind_index`, `created_by`, `clinic_id`, `identifier_system_identifiers`, `patient_id`, `patient_identifier_identifier_system`)
SELECT `id`, `created_at`, `updated_at`, `system`, `value_ciphertext`, `blind_index`, `created_by`, `clinic_id`, `identifier_system_identifiers`, `patient_id`, `patient_identifier_identifier_system`
FROM `patient_identifiers`;

DROP TABLE `patient_identifiers`;
ALTER TABLE `new_patient_identifiers` RENAME TO `patient_identifiers`;
CREATE INDEX `patientidentifier_patient_id` ON `patient_identifiers` (`patient_id`);
CREATE INDEX `patientidentifier_system` ON `patient_identifiers` (`system`);
CREATE UNIQUE INDEX `patientidentifier_clinic_id_blind_index` ON `patient_identifiers` (`clinic_id`, `blind_index`);

PRAGMA foreign_keys = on;

-- +goose Down
PRAGMA foreign_keys = off;

CREATE TABLE `new_patient_identifiers` (
  `id` uuid NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `system` text NOT NULL,
  `value_ciphertext` blob NOT NULL,
  `nonce` blob NOT NULL,
  `blind_index` text NOT NULL,
  `created_by` uuid NULL,
  `clinic_id` uuid NOT NULL,
  `identifier_system_identifiers` uuid NULL,
  `patient_id` uuid NOT NULL,
  `patient_identifier_identifier_system` uuid NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `patient_identifiers_clinics_patient_identifiers` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patient_identifiers_identifier_systems_identifiers` FOREIGN KEY (`identifier_system_identifiers`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL,
  CONSTRAINT `patient_identifiers_patients_identifiers` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patient_identifiers_identifier_systems_identifier_system` FOREIGN KEY (`patient_identifier_identifier_system`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL
);

INSERT INTO `new_patient_identifiers` (`id`, `created_at`, `updated_at`, `system`, `value_ciphertext`, `nonce`, `blind_index`, `created_by`, `clinic_id`, `identifier_system_identifiers`, `patient_id`, `patient_identifier_identifier_system`)
SELECT `id`, `created_at`, `updated_at`, `system`, `value_ciphertext`, x'', `blind_index`, `created_by`, `clinic_id`, `identifier_system_identifiers`, `patient_id`, `patient_identifier_identifier_system`
FROM `patient_identifiers`;

DROP TABLE `patient_identifiers`;
ALTER TABLE `new_patient_identifiers` RENAME TO `patient_identifiers`;
CREATE INDEX `patientidentifier_patient_id` ON `patient_identifiers` (`patient_id`);
CREATE INDEX `patientidentifier_system` ON `patient_identifiers` (`system`);
CREATE UNIQUE INDEX `patientidentifier_clinic_id_blind_index` ON `patient_identifiers` (`clinic_id`, `blind_index`);

PRAGMA foreign_keys = on;
