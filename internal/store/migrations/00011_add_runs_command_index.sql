-- +goose Up
CREATE INDEX IF NOT EXISTS idx_runs_command ON runs(command);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_command;
