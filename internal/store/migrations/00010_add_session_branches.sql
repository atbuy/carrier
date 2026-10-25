-- +goose Up
CREATE TABLE IF NOT EXISTS session_branches (
    git_root   TEXT NOT NULL,
    branch     TEXT NOT NULL,
    session_id INTEGER NOT NULL REFERENCES shell_sessions(id),
    linked_at  TEXT NOT NULL,
    PRIMARY KEY (git_root, branch)
);
CREATE INDEX IF NOT EXISTS idx_session_branches_session_id ON session_branches(session_id);

-- +goose Down
DROP INDEX IF EXISTS idx_session_branches_session_id;
DROP TABLE IF EXISTS session_branches;
