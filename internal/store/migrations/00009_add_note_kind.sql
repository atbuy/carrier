-- +goose Up
ALTER TABLE notes ADD COLUMN kind TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE notes DROP COLUMN kind;
