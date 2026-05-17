-- +goose Up
CREATE TABLE IF NOT EXISTS shell_sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    ended_at   TEXT
);

ALTER TABLE runs ADD COLUMN session_id INTEGER REFERENCES shell_sessions(id);

CREATE INDEX IF NOT EXISTS idx_runs_session_id ON runs(session_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_session_id;
DROP TABLE IF EXISTS shell_sessions;
