-- +goose Up
CREATE VIRTUAL TABLE IF NOT EXISTS run_search USING fts5(command, cwd, output);

INSERT INTO run_search(rowid, command, cwd, output)
SELECT id, command, cwd, ''
FROM runs
WHERE id NOT IN (SELECT rowid FROM run_search);

-- +goose Down
DROP TABLE IF EXISTS run_search;
