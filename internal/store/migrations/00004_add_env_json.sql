-- +goose Up
ALTER TABLE runs ADD COLUMN env_json TEXT;

-- +goose Down
