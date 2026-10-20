-- +goose Up
ALTER TABLE shell_sessions ADD COLUMN auto INTEGER NOT NULL DEFAULT 0;
ALTER TABLE shell_sessions ADD COLUMN kept INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE shell_sessions DROP COLUMN kept;
ALTER TABLE shell_sessions DROP COLUMN auto;
