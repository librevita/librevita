-- +goose Up
-- Multi-clinic isolation: clinic_id on identity tables, clinic slug,
-- platform operators, identifier opt-in, and clinic_id on the audit chain.
-- Existing rows are attached to the oldest clinic (fallback slug "default").
PRAGMA foreign_keys = off;

-- 1. Clinics: slug + onboarded_at
CREATE TABLE `new_clinics` (
  `id` uuid NOT NULL,
  `slug` text NOT NULL,
  `name` text NOT NULL,
  `tax_id` text NULL,
  `phone` text NULL,
  `email` text NULL,
  `street` text NULL,
  `city` text NULL,
  `state` text NULL,
  `postal_code` text NULL,
  `country` text NOT NULL DEFAULT ('BR'),
  `timezone` text NOT NULL DEFAULT ('America/Sao_Paulo'),
  `onboarded_at` datetime NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
);
INSERT INTO `new_clinics` (`id`, `slug`, `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`, `onboarded_at`, `created_at`, `updated_at`)
SELECT
  `id`,
  CASE
    WHEN `id` = (SELECT `id` FROM `clinics` ORDER BY `created_at` ASC LIMIT 1) THEN 'default'
    ELSE 'c-' || lower(substr(replace(`id`, '-', ''), 1, 12))
  END,
  `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`,
  `created_at`,
  `created_at`,
  `updated_at`
FROM `clinics`;
DROP TABLE `clinics`;
ALTER TABLE `new_clinics` RENAME TO `clinics`;
CREATE UNIQUE INDEX `clinics_slug_key` ON `clinics` (`slug`);

-- 2. Roles per clinic
CREATE TABLE `new_roles` (
  `id` uuid NOT NULL,
  `name` text NOT NULL,
  `system` bool NOT NULL DEFAULT (false),
  `is_clinical` bool NOT NULL DEFAULT (false),
  `created_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `roles_clinics_roles` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION
);
INSERT INTO `new_roles` (`id`, `name`, `system`, `is_clinical`, `created_at`, `clinic_id`)
SELECT `id`, `name`, `system`, `is_clinical`, `created_at`,
  (SELECT `id` FROM `clinics` ORDER BY `created_at` ASC LIMIT 1)
FROM `roles`;
DROP TABLE `roles`;
ALTER TABLE `new_roles` RENAME TO `roles`;
CREATE UNIQUE INDEX `role_clinic_id_name` ON `roles` (`clinic_id`, `name`);

-- 3. Users per clinic
CREATE TABLE `new_users` (
  `id` uuid NOT NULL,
  `email` text NOT NULL,
  `password_hash` text NOT NULL,
  `display_name` text NOT NULL,
  `active` bool NOT NULL DEFAULT (true),
  `timezone` text NOT NULL DEFAULT (''),
  `ui_theme` text NOT NULL DEFAULT ('system'),
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `role_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `users_clinics_users` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `users_roles_users` FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `users_ui_theme_check` CHECK (ui_theme IN ('system', 'light', 'dark'))
);
INSERT INTO `new_users` (`id`, `email`, `password_hash`, `display_name`, `active`, `timezone`, `ui_theme`, `created_at`, `updated_at`, `clinic_id`, `role_id`)
SELECT `id`, `email`, `password_hash`, `display_name`, `active`, `timezone`, `ui_theme`, `created_at`, `updated_at`,
  (SELECT `id` FROM `clinics` ORDER BY `created_at` ASC LIMIT 1),
  `role_id`
FROM `users`;
DROP TABLE `users`;
ALTER TABLE `new_users` RENAME TO `users`;
CREATE UNIQUE INDEX `user_clinic_id_email` ON `users` (`clinic_id`, `email`);

-- 4. Policies per clinic
CREATE TABLE `new_policies` (
  `id` uuid NOT NULL,
  `name` text NOT NULL,
  `expression` text NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `policies_clinics_policies` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION
);
INSERT INTO `new_policies` (`id`, `name`, `expression`, `updated_at`, `clinic_id`)
SELECT `id`, `name`, `expression`, `updated_at`,
  (SELECT `id` FROM `clinics` ORDER BY `created_at` ASC LIMIT 1)
FROM `policies`;
DROP TABLE `policies`;
ALTER TABLE `new_policies` RENAME TO `policies`;
CREATE UNIQUE INDEX `accesspolicy_clinic_id_name` ON `policies` (`clinic_id`, `name`);

-- 5. Patients.user_id (portal)
CREATE TABLE `new_patients` (
  `id` uuid NOT NULL,
  `status` text NOT NULL DEFAULT ('active'),
  `display_name` blob NOT NULL,
  `phone` blob NOT NULL,
  `email` blob NOT NULL,
  `birth_date` blob NULL,
  `sex` blob NULL,
  `street` blob NULL,
  `city` blob NULL,
  `state` blob NULL,
  `postal_code` blob NULL,
  `notes` blob NULL,
  `created_by` uuid NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `display_name_token_index` json NULL,
  `phone_blind_index` text NULL,
  `email_blind_index` text NULL,
  `clinic_id` uuid NOT NULL,
  `user_id` uuid NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `patients_clinics_patients` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patients_users_portal_patient` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `patients_status_check` CHECK (status IN ('active', 'inactive', 'archived'))
);
INSERT INTO `new_patients` (`id`, `status`, `display_name`, `phone`, `email`, `birth_date`, `sex`, `street`, `city`, `state`, `postal_code`, `notes`, `created_by`, `created_at`, `updated_at`, `display_name_token_index`, `phone_blind_index`, `email_blind_index`, `clinic_id`)
SELECT `id`, `status`, `display_name`, `phone`, `email`, `birth_date`, `sex`, `street`, `city`, `state`, `postal_code`, `notes`, `created_by`, `created_at`, `updated_at`, `display_name_token_index`, `phone_blind_index`, `email_blind_index`, `clinic_id`
FROM `patients`;
DROP TABLE `patients`;
ALTER TABLE `new_patients` RENAME TO `patients`;
CREATE INDEX `patient_clinic_id` ON `patients` (`clinic_id`);
CREATE INDEX `patient_status` ON `patients` (`status`);
CREATE UNIQUE INDEX `patient_clinic_id_user_id` ON `patients` (`clinic_id`, `user_id`) WHERE user_id IS NOT NULL;
CREATE INDEX `patient_clinic_id_phone_blind_index` ON `patients` (`clinic_id`, `phone_blind_index`);
CREATE INDEX `patient_clinic_id_email_blind_index` ON `patients` (`clinic_id`, `email_blind_index`);

-- 6. Patient identifiers: clinic_id + unique (clinic_id, blind_index)
CREATE TABLE `new_patient_identifiers` (
  `id` uuid NOT NULL,
  `system` text NOT NULL,
  `value_ciphertext` blob NOT NULL,
  `nonce` blob NOT NULL,
  `blind_index` text NOT NULL,
  `created_by` uuid NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
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
INSERT INTO `new_patient_identifiers` (`id`, `system`, `value_ciphertext`, `nonce`, `blind_index`, `created_by`, `created_at`, `updated_at`, `clinic_id`, `identifier_system_identifiers`, `patient_id`, `patient_identifier_identifier_system`)
SELECT pi.`id`, pi.`system`, pi.`value_ciphertext`, pi.`nonce`, pi.`blind_index`, pi.`created_by`, pi.`created_at`, pi.`updated_at`,
  p.`clinic_id`,
  pi.`identifier_system_identifiers`, pi.`patient_id`, pi.`patient_identifier_identifier_system`
FROM `patient_identifiers` pi
JOIN `patients` p ON p.`id` = pi.`patient_id`;
DROP TABLE `patient_identifiers`;
ALTER TABLE `new_patient_identifiers` RENAME TO `patient_identifiers`;
CREATE INDEX `patientidentifier_patient_id` ON `patient_identifiers` (`patient_id`);
CREATE INDEX `patientidentifier_system` ON `patient_identifiers` (`system`);
CREATE UNIQUE INDEX `patientidentifier_clinic_id_blind_index` ON `patient_identifiers` (`clinic_id`, `blind_index`);

-- 7. Staff change requests
CREATE TABLE `new_staff_change_requests` (
  `id` uuid NOT NULL,
  `status` text NOT NULL DEFAULT ('pending'),
  `changes` text NOT NULL,
  `decision_note` text NULL,
  `created_at` datetime NOT NULL,
  `decided_at` datetime NULL,
  `clinic_id` uuid NOT NULL,
  `requested_by` uuid NOT NULL,
  `decided_by` uuid NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `staff_change_requests_clinics_staff_requests` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `staff_change_requests_users_requester` FOREIGN KEY (`requested_by`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `staff_change_requests_users_decider` FOREIGN KEY (`decided_by`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `staff_change_requests_users_staff_requests` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `staff_change_requests_status_check` CHECK (status IN ('pending', 'approved', 'rejected'))
);
INSERT INTO `new_staff_change_requests` (`id`, `status`, `changes`, `decision_note`, `created_at`, `decided_at`, `clinic_id`, `requested_by`, `decided_by`, `user_id`)
SELECT scr.`id`, scr.`status`, scr.`changes`, scr.`decision_note`, scr.`created_at`, scr.`decided_at`,
  u.`clinic_id`,
  scr.`requested_by`, scr.`decided_by`, scr.`user_id`
FROM `staff_change_requests` scr
JOIN `users` u ON u.`id` = scr.`user_id`;
DROP TABLE `staff_change_requests`;
ALTER TABLE `new_staff_change_requests` RENAME TO `staff_change_requests`;
CREATE INDEX `staffchangerequest_clinic_id` ON `staff_change_requests` (`clinic_id`);
CREATE INDEX `staffchangerequest_status_created_at` ON `staff_change_requests` (`status`, `created_at`);
CREATE INDEX `staffchangerequest_user_id` ON `staff_change_requests` (`user_id`);
CREATE INDEX `staffchangerequest_requested_by_created_at` ON `staff_change_requests` (`requested_by`, `created_at`);

-- 8. Storage objects
CREATE TABLE `new_storage_objects` (
  `id` uuid NOT NULL,
  `key` text NOT NULL,
  `domain` text NOT NULL,
  `resource_id` text NOT NULL,
  `original_name` text NOT NULL,
  `content_type` text NOT NULL,
  `size` integer NOT NULL,
  `etag` text NOT NULL,
  `checksum` text NOT NULL,
  `created_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `created_by` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `storage_objects_clinics_storage_objects` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `storage_objects_users_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE NO ACTION
);
INSERT INTO `new_storage_objects` (`id`, `key`, `domain`, `resource_id`, `original_name`, `content_type`, `size`, `etag`, `checksum`, `created_at`, `clinic_id`, `created_by`)
SELECT so.`id`, so.`key`, so.`domain`, so.`resource_id`, so.`original_name`, so.`content_type`, so.`size`, so.`etag`, so.`checksum`, so.`created_at`,
  u.`clinic_id`,
  so.`created_by`
FROM `storage_objects` so
JOIN `users` u ON u.`id` = so.`created_by`;
DROP TABLE `storage_objects`;
ALTER TABLE `new_storage_objects` RENAME TO `storage_objects`;
CREATE UNIQUE INDEX `storage_objects_key_key` ON `storage_objects` (`key`);
CREATE INDEX `storageobject_clinic_id` ON `storage_objects` (`clinic_id`);
CREATE INDEX `storageobject_domain_resource_id_created_at` ON `storage_objects` (`domain`, `resource_id`, `created_at`);

-- 9. Audit log: nullable clinic_id (chain payload includes it)
CREATE TABLE `new_audit_log` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `actor_id` text NULL,
  `actor_email` text NULL,
  `action` text NOT NULL,
  `resource` text NOT NULL,
  `result` text NOT NULL,
  `ip` text NULL,
  `request_id` text NULL,
  `detail` text NULL,
  `actor_name` text NOT NULL DEFAULT (''),
  `actor_role` text NOT NULL DEFAULT (''),
  `user_agent` text NOT NULL DEFAULT (''),
  `resource_name` text NOT NULL DEFAULT (''),
  `created_at` datetime NOT NULL,
  `signature` text NOT NULL,
  `clinic_id` uuid NULL,
  CONSTRAINT `audit_log_clinics_audit_logs` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE SET NULL,
  CONSTRAINT `audit_log_result_check` CHECK (result IN ('success', 'failure'))
);
INSERT INTO `new_audit_log` (`id`, `actor_id`, `actor_email`, `action`, `resource`, `result`, `ip`, `request_id`, `detail`, `actor_name`, `actor_role`, `user_agent`, `resource_name`, `created_at`, `signature`, `clinic_id`)
SELECT `id`, `actor_id`, `actor_email`, `action`, `resource`, `result`, `ip`, `request_id`, `detail`, `actor_name`, `actor_role`, `user_agent`, `resource_name`, `created_at`, `signature`, NULL
FROM `audit_log`;
DROP TABLE `audit_log`;
ALTER TABLE `new_audit_log` RENAME TO `audit_log`;
CREATE INDEX `auditlog_actor_id` ON `audit_log` (`actor_id`);
CREATE INDEX `auditlog_action` ON `audit_log` (`action`);
CREATE INDEX `auditlog_created_at` ON `audit_log` (`created_at`);
CREATE INDEX `auditlog_resource_id` ON `audit_log` (`resource`, `id`);
CREATE INDEX `auditlog_clinic_id_id` ON `audit_log` (`clinic_id`, `id`);

-- 10. Platform operators (apex)
CREATE TABLE `platform_users` (
  `id` uuid NOT NULL,
  `email` text NOT NULL,
  `password_hash` text NOT NULL,
  `display_name` text NOT NULL,
  `active` bool NOT NULL DEFAULT (true),
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
);
CREATE UNIQUE INDEX `platform_users_email_key` ON `platform_users` (`email`);

CREATE TABLE `platform_sessions` (
  `token_hash` text NOT NULL,
  `expires_at` datetime NOT NULL,
  `platform_user_id` uuid NOT NULL,
  PRIMARY KEY (`token_hash`),
  CONSTRAINT `platform_sessions_platform_users_sessions` FOREIGN KEY (`platform_user_id`) REFERENCES `platform_users` (`id`) ON DELETE NO ACTION
);
CREATE INDEX `platformsession_platform_user_id` ON `platform_sessions` (`platform_user_id`);
CREATE INDEX `platformsession_expires_at` ON `platform_sessions` (`expires_at`);

-- 11. Clinic opt-in to the global identifier catalog
CREATE TABLE `clinic_identifier_systems` (
  `id` uuid NOT NULL,
  `clinic_id` uuid NOT NULL,
  `identifier_system_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `clinic_identifier_systems_clinics_identifier_systems` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `clinic_identifier_systems_identifier_systems_clinic_opt_ins` FOREIGN KEY (`identifier_system_id`) REFERENCES `identifier_systems` (`id`) ON DELETE NO ACTION
);
CREATE UNIQUE INDEX `clinicidentifiersystem_clinic_id_identifier_system_id` ON `clinic_identifier_systems` (`clinic_id`, `identifier_system_id`);
INSERT INTO `clinic_identifier_systems` (`id`, `clinic_id`, `identifier_system_id`)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-7' || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(6))),
  c.`id`,
  s.`id`
FROM `clinics` c
CROSS JOIN `identifier_systems` s
WHERE s.`active` = 1;

PRAGMA foreign_keys = on;

-- +goose Down
SELECT RAISE(ABORT, 'multi_clinic down is not supported');
