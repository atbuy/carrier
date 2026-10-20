-- +goose Up
CREATE TABLE IF NOT EXISTS notes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER REFERENCES shell_sessions(id),
    run_id     INTEGER REFERENCES runs(id),
    text       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_session_id ON notes(session_id);
CREATE INDEX IF NOT EXISTS idx_notes_run_id ON notes(run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_notes_run_id;
DROP INDEX IF EXISTS idx_notes_session_id;
DROP TABLE IF EXISTS notes;
