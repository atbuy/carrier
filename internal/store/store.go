package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "runs"), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "carrier.db"))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateRun(r CreateRun) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO runs
(status, mode, command, argv_json, cwd, started_at, hostname, shell, git_root, git_branch, git_commit, git_dirty, stdout_path, stderr_path, terminal_output_path, notify_requested, notify_always)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Status, r.Mode, r.Command, r.ArgvJSON, r.CWD, fmtTime(r.StartedAt), r.Hostname, r.Shell,
		r.GitRoot, r.GitBranch, r.GitCommit, nullableBool(r.GitDirty), r.StdoutPath, r.StderrPath,
		r.TerminalOutputPath, boolInt(r.NotifyRequested), boolInt(r.NotifyAlways))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdatePaths(id int64, stdoutPath, stderrPath, terminalPath string) error {
	_, err := s.db.Exec(`UPDATE runs SET stdout_path=?, stderr_path=?, terminal_output_path=? WHERE id=?`, stdoutPath, stderrPath, terminalPath, id)
	return err
}

func (s *Store) FinishRun(id int64, status string, exitCode int, finished time.Time) error {
	r, err := s.GetRun(id)
	if err != nil {
		return err
	}
	duration := finished.Sub(r.StartedAt).Milliseconds()
	_, err = s.db.Exec(`UPDATE runs SET status=?, finished_at=?, duration_ms=?, exit_code=? WHERE id=?`,
		status, fmtTime(finished), duration, exitCode, id)
	return err
}

func (s *Store) GetRun(id int64) (*Run, error) {
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at FROM runs WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRun(rows)
}

func (s *Store) Latest() (*Run, error) {
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at FROM runs ORDER BY id DESC LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRun(rows)
}

func (s *Store) ListByStatus(status string, limit int) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at FROM runs WHERE status=? ORDER BY id DESC LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

func (s *Store) All(limit int) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at FROM runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

func (s *Store) DeleteOlderThan(cutoff time.Time) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at FROM runs WHERE started_at < ?`, fmtTime(cutoff))
	if err != nil {
		return nil, err
	}
	runs, err := scanRuns(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM runs WHERE started_at < ?`, fmtTime(cutoff)); err != nil {
		return nil, err
	}
	return runs, nil
}

func scanRuns(rows *sql.Rows) ([]Run, error) {
	var runs []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(rows rowScanner) (*Run, error) {
	var r Run
	var started, finished, created sql.NullString
	var duration sql.NullInt64
	var exitCode sql.NullInt64
	var dirty sql.NullInt64
	var notifyRequested, notifyAlways int
	err := rows.Scan(&r.ID, &r.Status, &r.Mode, &r.Command, &r.ArgvJSON, &r.CWD, &started, &finished,
		&duration, &exitCode, &r.Hostname, &r.Shell, &r.GitRoot, &r.GitBranch, &r.GitCommit, &dirty,
		&r.StdoutPath, &r.StderrPath, &r.TerminalOutputPath, &notifyRequested, &notifyAlways, &created)
	if err != nil {
		return nil, err
	}
	r.StartedAt, _ = parseTime(started.String)
	if finished.Valid {
		t, _ := parseTime(finished.String)
		r.FinishedAt = &t
	}
	if duration.Valid {
		v := duration.Int64
		r.DurationMS = &v
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		r.ExitCode = &v
	}
	if dirty.Valid {
		v := dirty.Int64 != 0
		r.GitDirty = &v
	}
	r.NotifyRequested = notifyRequested != 0
	r.NotifyAlways = notifyAlways != 0
	r.CreatedAt, _ = parseTime(created.String)
	return &r, nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	return time.Parse(time.RFC3339Nano, s)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableBool(v *bool) any {
	if v == nil {
		return nil
	}
	return boolInt(*v)
}
