-- +goose Up
-- +goose NO TRANSACTION
-- Initial baseline. No business tables here; persist WAL mode in the
-- database file. All tables are created STRICT (SQLite strict typing):
-- columns accept only their declared affinity, so a wrong type is a
-- hard error instead of silent coercion.
PRAGMA journal_mode = WAL;

-- +goose Down
-- +goose NO TRANSACTION
PRAGMA journal_mode = DELETE;
