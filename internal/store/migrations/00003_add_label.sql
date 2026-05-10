-- +goose Up
ALTER TABLE runs ADD COLUMN label TEXT;

CREATE INDEX IF NOT EXISTS idx_runs_label ON runs(label);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_label;
