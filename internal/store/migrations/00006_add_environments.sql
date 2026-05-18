-- +goose Up
CREATE TABLE IF NOT EXISTS environments (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    hash     TEXT NOT NULL UNIQUE,
    env_json TEXT NOT NULL
);

ALTER TABLE runs ADD COLUMN env_id INTEGER REFERENCES environments(id);

CREATE INDEX IF NOT EXISTS idx_runs_env_id ON runs(env_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_env_id;
ALTER TABLE runs DROP COLUMN env_id;
DROP TABLE IF EXISTS environments;
