-- +goose Up
-- +goose NO TRANSACTION
-- Initial database baseline.
-- No business tables are defined here. Persist WAL mode in the database file.
PRAGMA journal_mode = WAL;

-- +goose Down
-- +goose NO TRANSACTION
PRAGMA journal_mode = DELETE;
