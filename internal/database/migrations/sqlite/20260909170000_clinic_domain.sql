-- +goose Up
-- disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;

-- create "new_clinics" table
CREATE TABLE `new_clinics` (
  `id` uuid NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  `domain` text NOT NULL,
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
  PRIMARY KEY (`id`)
);

-- copy rows from old table "clinics" to new temporary table "new_clinics"
INSERT INTO `new_clinics` (`id`, `created_at`, `updated_at`, `domain`, `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`, `onboarded_at`) SELECT `id`, `created_at`, `updated_at`, `slug`, `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`, `onboarded_at` FROM `clinics`;

-- drop "clinics" table after copying rows
DROP TABLE `clinics`;

-- rename temporary table "new_clinics" to "clinics"
ALTER TABLE `new_clinics` RENAME TO `clinics`;

-- create index "clinics_domain_key" to table: "clinics"
CREATE UNIQUE INDEX `clinics_domain_key` ON `clinics` (`domain`);

-- enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;

-- +goose Down
PRAGMA foreign_keys = off;
CREATE TABLE `new_clinics` (
  `id` uuid NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
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
  PRIMARY KEY (`id`)
);
INSERT INTO `new_clinics` (`id`, `created_at`, `updated_at`, `slug`, `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`, `onboarded_at`) SELECT `id`, `created_at`, `updated_at`, `domain`, `name`, `tax_id`, `phone`, `email`, `street`, `city`, `state`, `postal_code`, `country`, `timezone`, `onboarded_at` FROM `clinics`;
DROP TABLE `clinics`;
ALTER TABLE `new_clinics` RENAME TO `clinics`;
CREATE UNIQUE INDEX `clinics_slug_key` ON `clinics` (`slug`);
PRAGMA foreign_keys = on;
