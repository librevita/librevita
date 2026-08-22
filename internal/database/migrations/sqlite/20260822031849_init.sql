-- +goose Up
-- create "policies" table
CREATE TABLE `policies` (
  `id` uuid NOT NULL,
  `name` text NOT NULL,
  `expression` text NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
);

-- create index "policies_name_key" to table: "policies"
CREATE UNIQUE INDEX `policies_name_key` ON `policies` (`name`);

-- create "policy_versions" table
CREATE TABLE `policy_versions` (
  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  `expression` text NOT NULL,
  `changed_by` text NULL,
  `changed_by_email` text NULL,
  `origin` text NOT NULL DEFAULT ('system'),
  `created_at` datetime NOT NULL,
  `policy_id` uuid NOT NULL,
  CONSTRAINT `policy_versions_policies_versions` FOREIGN KEY (`policy_id`) REFERENCES `policies` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `policy_versions_origin_check` CHECK (origin IN ('seed', 'admin', 'system'))
);

-- create index "accesspolicyversion_policy_id_id" to table: "policy_versions"
CREATE INDEX `accesspolicyversion_policy_id_id` ON `policy_versions` (`policy_id`, `id`);

-- create "appointments" table
CREATE TABLE `appointments` (
  `id` uuid NOT NULL,
  `start_time` datetime NOT NULL,
  `end_time` datetime NOT NULL,
  `status` text NOT NULL DEFAULT ('scheduled'),
  `reason` blob NULL,
  `notes` blob NULL,
  `created_by` uuid NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  `patient_id` uuid NOT NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `appointments_clinics_appointments` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `appointments_patients_appointments` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `appointments_users_appointments` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `appointments_status_check` CHECK (status IN ('scheduled', 'confirmed', 'in_progress', 'completed', 'cancelled', 'no_show'))
);

-- create index "appointment_clinic_id_start_time" to table: "appointments"
CREATE INDEX `appointment_clinic_id_start_time` ON `appointments` (`clinic_id`, `start_time`);

-- create index "appointment_user_id_start_time" to table: "appointments"
CREATE INDEX `appointment_user_id_start_time` ON `appointments` (`user_id`, `start_time`);

-- create index "appointment_patient_id_start_time" to table: "appointments"
CREATE INDEX `appointment_patient_id_start_time` ON `appointments` (`patient_id`, `start_time`);

-- create index "appointment_status" to table: "appointments"
CREATE INDEX `appointment_status` ON `appointments` (`status`);

-- create "audit_log" table
CREATE TABLE `audit_log` (
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
  CONSTRAINT `audit_log_result_check` CHECK (result IN ('success', 'failure'))
);

-- create index "auditlog_actor_id" to table: "audit_log"
CREATE INDEX `auditlog_actor_id` ON `audit_log` (`actor_id`);

-- create index "auditlog_action" to table: "audit_log"
CREATE INDEX `auditlog_action` ON `audit_log` (`action`);

-- create index "auditlog_created_at" to table: "audit_log"
CREATE INDEX `auditlog_created_at` ON `audit_log` (`created_at`);

-- create index "auditlog_resource_id" to table: "audit_log"
CREATE INDEX `auditlog_resource_id` ON `audit_log` (`resource`, `id`);

-- create "clinics" table
CREATE TABLE `clinics` (
  `id` uuid NOT NULL,
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
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
);

-- create "episodes" table
CREATE TABLE `episodes` (
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

-- create "identifier_systems" table
CREATE TABLE `identifier_systems` (
  `id` uuid NOT NULL,
  `system` text NOT NULL,
  `display_name` text NOT NULL,
  `pattern` text NOT NULL,
  `transform` text NOT NULL DEFAULT ('none'),
  `check_algorithm` text NOT NULL DEFAULT ('none'),
  `check_base_len` integer NOT NULL DEFAULT (0),
  `check_dv_count` integer NOT NULL DEFAULT (1),
  `check_start_weight` integer NOT NULL DEFAULT (10),
  `active` bool NOT NULL DEFAULT (true),
  `mask` text NOT NULL DEFAULT (''),
  `created_by` uuid NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `identifier_systems_check_algorithm_check` CHECK (check_algorithm IN ('none', 'mod11_desc', 'mod11_cyclic')),
  CONSTRAINT `identifier_systems_transform_check` CHECK (transform IN ('none', 'digits', 'upper', 'lower'))
);

-- create index "identifier_systems_system_key" to table: "identifier_systems"
CREATE UNIQUE INDEX `identifier_systems_system_key` ON `identifier_systems` (`system`);

-- create "meta" table
CREATE TABLE `meta` (
  `key` text NOT NULL,
  `value` text NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`key`)
);

-- create "patients" table
CREATE TABLE `patients` (
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
  `display_name_blind_index` text NULL,
  `phone_blind_index` text NULL,
  `email_blind_index` text NULL,
  `clinic_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `patients_clinics_patients` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patients_status_check` CHECK (status IN ('active', 'inactive', 'archived'))
);

-- create index "patient_clinic_id" to table: "patients"
CREATE INDEX `patient_clinic_id` ON `patients` (`clinic_id`);

-- create index "patient_status" to table: "patients"
CREATE INDEX `patient_status` ON `patients` (`status`);

-- create index "patient_clinic_id_display_name_blind_index" to table: "patients"
CREATE INDEX `patient_clinic_id_display_name_blind_index` ON `patients` (`clinic_id`, `display_name_blind_index`);

-- create index "patient_clinic_id_phone_blind_index" to table: "patients"
CREATE INDEX `patient_clinic_id_phone_blind_index` ON `patients` (`clinic_id`, `phone_blind_index`);

-- create index "patient_clinic_id_email_blind_index" to table: "patients"
CREATE INDEX `patient_clinic_id_email_blind_index` ON `patients` (`clinic_id`, `email_blind_index`);

-- create "patient_identifiers" table
CREATE TABLE `patient_identifiers` (
  `id` uuid NOT NULL,
  `system` text NOT NULL,
  `value_ciphertext` blob NOT NULL,
  `nonce` blob NOT NULL,
  `blind_index` text NOT NULL,
  `created_by` uuid NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `identifier_system_identifiers` uuid NULL,
  `patient_id` uuid NOT NULL,
  `patient_identifier_identifier_system` uuid NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `patient_identifiers_identifier_systems_identifiers` FOREIGN KEY (`identifier_system_identifiers`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL,
  CONSTRAINT `patient_identifiers_patients_identifiers` FOREIGN KEY (`patient_id`) REFERENCES `patients` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `patient_identifiers_identifier_systems_identifier_system` FOREIGN KEY (`patient_identifier_identifier_system`) REFERENCES `identifier_systems` (`id`) ON DELETE SET NULL
);

-- create index "patient_identifiers_blind_index_key" to table: "patient_identifiers"
CREATE UNIQUE INDEX `patient_identifiers_blind_index_key` ON `patient_identifiers` (`blind_index`);

-- create index "patientidentifier_patient_id" to table: "patient_identifiers"
CREATE INDEX `patientidentifier_patient_id` ON `patient_identifiers` (`patient_id`);

-- create index "patientidentifier_system" to table: "patient_identifiers"
CREATE INDEX `patientidentifier_system` ON `patient_identifiers` (`system`);

-- create "roles" table
CREATE TABLE `roles` (
  `id` uuid NOT NULL,
  `name` text NOT NULL,
  `system` bool NOT NULL DEFAULT (false),
  `is_clinical` bool NOT NULL DEFAULT (false),
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
);

-- create index "roles_name_key" to table: "roles"
CREATE UNIQUE INDEX `roles_name_key` ON `roles` (`name`);

-- create "sessions" table
CREATE TABLE `sessions` (
  `token_hash` text NOT NULL,
  `expires_at` datetime NOT NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`token_hash`),
  CONSTRAINT `sessions_users_sessions` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION
);

-- create index "session_user_id" to table: "sessions"
CREATE INDEX `session_user_id` ON `sessions` (`user_id`);

-- create index "session_expires_at" to table: "sessions"
CREATE INDEX `session_expires_at` ON `sessions` (`expires_at`);

-- create "specialties" table
CREATE TABLE `specialties` (
  `id` uuid NOT NULL,
  `name` text NOT NULL,
  `created_at` datetime NOT NULL,
  `clinic_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `specialties_clinics_specialties` FOREIGN KEY (`clinic_id`) REFERENCES `clinics` (`id`) ON DELETE NO ACTION
);

-- create index "specialty_clinic_id_name" to table: "specialties"
CREATE UNIQUE INDEX `specialty_clinic_id_name` ON `specialties` (`clinic_id`, `name`);

-- create "staff_change_requests" table
CREATE TABLE `staff_change_requests` (
  `id` uuid NOT NULL,
  `status` text NOT NULL DEFAULT ('pending'),
  `changes` text NOT NULL,
  `decision_note` text NULL,
  `created_at` datetime NOT NULL,
  `decided_at` datetime NULL,
  `requested_by` uuid NOT NULL,
  `decided_by` uuid NULL,
  `user_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `staff_change_requests_users_requester` FOREIGN KEY (`requested_by`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `staff_change_requests_users_decider` FOREIGN KEY (`decided_by`) REFERENCES `users` (`id`) ON DELETE SET NULL,
  CONSTRAINT `staff_change_requests_users_staff_requests` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `staff_change_requests_status_check` CHECK (status IN ('pending', 'approved', 'rejected'))
);

-- create index "staffchangerequest_status_created_at" to table: "staff_change_requests"
CREATE INDEX `staffchangerequest_status_created_at` ON `staff_change_requests` (`status`, `created_at`);

-- create index "staffchangerequest_user_id" to table: "staff_change_requests"
CREATE INDEX `staffchangerequest_user_id` ON `staff_change_requests` (`user_id`);

-- create index "staffchangerequest_requested_by_created_at" to table: "staff_change_requests"
CREATE INDEX `staffchangerequest_requested_by_created_at` ON `staff_change_requests` (`requested_by`, `created_at`);

-- create "storage_objects" table
CREATE TABLE `storage_objects` (
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
  `created_by` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `storage_objects_users_creator` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE NO ACTION
);

-- create index "storage_objects_key_key" to table: "storage_objects"
CREATE UNIQUE INDEX `storage_objects_key_key` ON `storage_objects` (`key`);

-- create index "storageobject_domain_resource_id_created_at" to table: "storage_objects"
CREATE INDEX `storageobject_domain_resource_id_created_at` ON `storage_objects` (`domain`, `resource_id`, `created_at`);

-- create "users" table
CREATE TABLE `users` (
  `id` uuid NOT NULL,
  `email` text NOT NULL,
  `password_hash` text NOT NULL,
  `display_name` text NOT NULL,
  `active` bool NOT NULL DEFAULT (true),
  `timezone` text NOT NULL DEFAULT (''),
  `ui_theme` text NOT NULL DEFAULT ('system'),
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `role_id` uuid NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `users_roles_users` FOREIGN KEY (`role_id`) REFERENCES `roles` (`id`) ON DELETE NO ACTION,
  CONSTRAINT `users_ui_theme_check` CHECK (ui_theme IN ('system', 'light', 'dark'))
);

-- create index "users_email_key" to table: "users"
CREATE UNIQUE INDEX `users_email_key` ON `users` (`email`);

-- create "user_specialties" table
CREATE TABLE `user_specialties` (
  `user_id` uuid NOT NULL,
  `specialty_id` uuid NOT NULL,
  PRIMARY KEY (`user_id`, `specialty_id`),
  CONSTRAINT `user_specialties_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
  CONSTRAINT `user_specialties_specialty_id` FOREIGN KEY (`specialty_id`) REFERENCES `specialties` (`id`) ON DELETE CASCADE
);

-- +goose Down
-- reverse: create "user_specialties" table
DROP TABLE `user_specialties`;
-- reverse: create index "users_email_key" to table: "users"
DROP INDEX `users_email_key`;
-- reverse: create "users" table
DROP TABLE `users`;
-- reverse: create index "storageobject_domain_resource_id_created_at" to table: "storage_objects"
DROP INDEX `storageobject_domain_resource_id_created_at`;
-- reverse: create index "storage_objects_key_key" to table: "storage_objects"
DROP INDEX `storage_objects_key_key`;
-- reverse: create "storage_objects" table
DROP TABLE `storage_objects`;
-- reverse: create index "staffchangerequest_requested_by_created_at" to table: "staff_change_requests"
DROP INDEX `staffchangerequest_requested_by_created_at`;
-- reverse: create index "staffchangerequest_user_id" to table: "staff_change_requests"
DROP INDEX `staffchangerequest_user_id`;
-- reverse: create index "staffchangerequest_status_created_at" to table: "staff_change_requests"
DROP INDEX `staffchangerequest_status_created_at`;
-- reverse: create "staff_change_requests" table
DROP TABLE `staff_change_requests`;
-- reverse: create index "specialty_clinic_id_name" to table: "specialties"
DROP INDEX `specialty_clinic_id_name`;
-- reverse: create "specialties" table
DROP TABLE `specialties`;
-- reverse: create index "session_expires_at" to table: "sessions"
DROP INDEX `session_expires_at`;
-- reverse: create index "session_user_id" to table: "sessions"
DROP INDEX `session_user_id`;
-- reverse: create "sessions" table
DROP TABLE `sessions`;
-- reverse: create index "roles_name_key" to table: "roles"
DROP INDEX `roles_name_key`;
-- reverse: create "roles" table
DROP TABLE `roles`;
-- reverse: create index "patientidentifier_system" to table: "patient_identifiers"
DROP INDEX `patientidentifier_system`;
-- reverse: create index "patientidentifier_patient_id" to table: "patient_identifiers"
DROP INDEX `patientidentifier_patient_id`;
-- reverse: create index "patient_identifiers_blind_index_key" to table: "patient_identifiers"
DROP INDEX `patient_identifiers_blind_index_key`;
-- reverse: create "patient_identifiers" table
DROP TABLE `patient_identifiers`;
-- reverse: create index "patient_clinic_id_email_blind_index" to table: "patients"
DROP INDEX `patient_clinic_id_email_blind_index`;
-- reverse: create index "patient_clinic_id_phone_blind_index" to table: "patients"
DROP INDEX `patient_clinic_id_phone_blind_index`;
-- reverse: create index "patient_clinic_id_display_name_blind_index" to table: "patients"
DROP INDEX `patient_clinic_id_display_name_blind_index`;
-- reverse: create index "patient_status" to table: "patients"
DROP INDEX `patient_status`;
-- reverse: create index "patient_clinic_id" to table: "patients"
DROP INDEX `patient_clinic_id`;
-- reverse: create "patients" table
DROP TABLE `patients`;
-- reverse: create "meta" table
DROP TABLE `meta`;
-- reverse: create index "identifier_systems_system_key" to table: "identifier_systems"
DROP INDEX `identifier_systems_system_key`;
-- reverse: create "identifier_systems" table
DROP TABLE `identifier_systems`;
-- reverse: create index "episode_status" to table: "episodes"
DROP INDEX `episode_status`;
-- reverse: create index "episode_episode_type" to table: "episodes"
DROP INDEX `episode_episode_type`;
-- reverse: create index "episode_user_id_created_at" to table: "episodes"
DROP INDEX `episode_user_id_created_at`;
-- reverse: create index "episode_patient_id_created_at" to table: "episodes"
DROP INDEX `episode_patient_id_created_at`;
-- reverse: create index "episode_clinic_id_created_at" to table: "episodes"
DROP INDEX `episode_clinic_id_created_at`;
-- reverse: create "episodes" table
DROP TABLE `episodes`;
-- reverse: create "clinics" table
DROP TABLE `clinics`;
-- reverse: create index "auditlog_resource_id" to table: "audit_log"
DROP INDEX `auditlog_resource_id`;
-- reverse: create index "auditlog_created_at" to table: "audit_log"
DROP INDEX `auditlog_created_at`;
-- reverse: create index "auditlog_action" to table: "audit_log"
DROP INDEX `auditlog_action`;
-- reverse: create index "auditlog_actor_id" to table: "audit_log"
DROP INDEX `auditlog_actor_id`;
-- reverse: create "audit_log" table
DROP TABLE `audit_log`;
-- reverse: create index "appointment_status" to table: "appointments"
DROP INDEX `appointment_status`;
-- reverse: create index "appointment_patient_id_start_time" to table: "appointments"
DROP INDEX `appointment_patient_id_start_time`;
-- reverse: create index "appointment_user_id_start_time" to table: "appointments"
DROP INDEX `appointment_user_id_start_time`;
-- reverse: create index "appointment_clinic_id_start_time" to table: "appointments"
DROP INDEX `appointment_clinic_id_start_time`;
-- reverse: create "appointments" table
DROP TABLE `appointments`;
-- reverse: create index "accesspolicyversion_policy_id_id" to table: "policy_versions"
DROP INDEX `accesspolicyversion_policy_id_id`;
-- reverse: create "policy_versions" table
DROP TABLE `policy_versions`;
-- reverse: create index "policies_name_key" to table: "policies"
DROP INDEX `policies_name_key`;
-- reverse: create "policies" table
DROP TABLE `policies`;
