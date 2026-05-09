package store

const schema = `
CREATE TABLE IF NOT EXISTS runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status TEXT NOT NULL,
    mode TEXT NOT NULL,
    command TEXT NOT NULL,
    argv_json TEXT NOT NULL,
    cwd TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    duration_ms INTEGER,
    exit_code INTEGER,
    hostname TEXT,
    shell TEXT,
    git_root TEXT,
    git_branch TEXT,
    git_commit TEXT,
    git_dirty INTEGER,
    stdout_path TEXT,
    stderr_path TEXT,
    terminal_output_path TEXT,
    notify_requested INTEGER NOT NULL DEFAULT 0,
    notify_always INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at);
`
