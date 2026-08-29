-- +goose Up
ALTER TABLE `episodes` ADD COLUMN `predecessor_id` uuid NULL;
CREATE UNIQUE INDEX `episode_predecessor_id` ON `episodes` (`predecessor_id`);

-- +goose Down
-- SQLite cannot drop a column used by a unique index without a table rebuild.
DROP INDEX IF EXISTS `episode_predecessor_id`;
