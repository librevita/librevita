-- +goose Up
-- +goose NO TRANSACTION
-- Per-user preferences: the UI color scheme and the personal timezone.
-- An empty timezone inherits the clinic timezone (the display clock
-- falls back to clinics.timezone); ui_theme mirrors the UITheme enum in
-- internal/types.
ALTER TABLE users ADD COLUMN timezone TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN ui_theme TEXT NOT NULL DEFAULT 'system'
CHECK (ui_theme IN ('system', 'light', 'dark'));

-- +goose Down
-- +goose NO TRANSACTION
ALTER TABLE users DROP COLUMN ui_theme;
ALTER TABLE users DROP COLUMN timezone;
