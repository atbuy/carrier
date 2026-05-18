package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/logs"
)

// runCols and runFrom are used by every run SELECT to keep queries consistent.
// All run queries LEFT JOIN environments so Run.EnvJSON is always populated.
const (
	runCols   = `r.id,r.status,r.mode,r.command,r.argv_json,r.cwd,r.started_at,r.finished_at,r.duration_ms,r.exit_code,r.hostname,r.shell,r.git_root,r.git_branch,r.git_commit,r.git_dirty,r.stdout_path,r.stderr_path,r.terminal_output_path,r.notify_requested,r.notify_always,r.created_at,r.label,e.env_json,r.session_id`
	runFrom   = `FROM runs r LEFT JOIN environments e ON r.env_id=e.id`
	runSelect = `SELECT ` + runCols + ` ` + runFrom
)

type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	return OpenWith(dataDir, config.Config{})
}

// OpenWith opens (or creates) the store and runs all migrations, including the
// Go-level env backfill which redacts values using cfg before writing to disk.
func OpenWith(dataDir string, cfg config.Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "runs"), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "carrier.db"))
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Always redact env values regardless of cfg.Redaction.Enabled.
	redactor := logs.NewRedactor(true, append(logs.BuiltinPatterns(), cfg.Redaction.Patterns...))
	if err := migrateEnvData(db, redactor); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) MigrationVersion() (int64, error) {
	var version int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1`).Scan(&version)
	return version, err
}

func (s *Store) CreateRun(r CreateRun) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO runs
(status, mode, command, argv_json, cwd, started_at, hostname, shell, git_root, git_branch, git_commit, git_dirty, stdout_path, stderr_path, terminal_output_path, notify_requested, notify_always, env_id, session_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Status, r.Mode, r.Command, r.ArgvJSON, r.CWD, fmtTime(r.StartedAt), r.Hostname, r.Shell,
		r.GitRoot, r.GitBranch, r.GitCommit, nullableBool(r.GitDirty), r.StdoutPath, r.StderrPath,
		r.TerminalOutputPath, boolInt(r.NotifyRequested), boolInt(r.NotifyAlways), nullableInt64(r.EnvID), nullableInt64(r.SessionID))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.upsertSearch(id, r.Command, r.CWD, ""); err != nil {
		return 0, err
	}
	return id, nil
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
	if err != nil {
		return err
	}
	return s.indexRun(id)
}

func (s *Store) GetRun(id int64) (*Run, error) {
	rows, err := s.db.Query(`SELECT `+runCols+` `+runFrom+` WHERE r.id=?`, id)
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
	rows, err := s.db.Query(runSelect + ` ORDER BY r.id DESC LIMIT 1`)
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
	rows, err := s.db.Query(runSelect+` WHERE r.status=? ORDER BY r.id DESC LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

func (s *Store) All(limit int) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` ORDER BY r.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

// AllRuns returns every run ordered by id descending with no limit.
func (s *Store) AllRuns() ([]Run, error) {
	rows, err := s.db.Query(runSelect + ` ORDER BY r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

// HistoryFilter holds optional filter criteria for ListHistory.
// Zero values mean "no filter" for that field.
type HistoryFilter struct {
	Status    string    // exact match on status column
	Since     time.Time // only runs started at or after this time
	CWD       string    // substring match on cwd
	Branch    string    // exact match on git_branch
	Command   string    // substring match on command
	Label     string    // substring match on label
	SessionID *int64    // exact match on session_id
}

func (s *Store) ListHistory(limit int, f HistoryFilter) ([]Run, error) {
	where := []string{}
	args := []any{}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if !f.Since.IsZero() {
		where = append(where, "started_at >= ?")
		args = append(args, fmtTime(f.Since))
	}
	if f.CWD != "" {
		where = append(where, "cwd LIKE ?")
		args = append(args, "%"+f.CWD+"%")
	}
	if f.Branch != "" {
		where = append(where, "git_branch = ?")
		args = append(args, f.Branch)
	}
	if f.Command != "" {
		where = append(where, "command LIKE ?")
		args = append(args, "%"+f.Command+"%")
	}
	if f.Label != "" {
		where = append(where, "label LIKE ?")
		args = append(args, "%"+f.Label+"%")
	}
	if f.SessionID != nil {
		where = append(where, "session_id = ?")
		args = append(args, *f.SessionID)
	}
	q := runSelect
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY r.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

func (s *Store) ListOlderThan(cutoff time.Time) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.started_at < ? ORDER BY r.id`, fmtTime(cutoff))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

func (s *Store) DeleteOlderThan(cutoff time.Time) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.started_at < ?`, fmtTime(cutoff))
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
	if err := s.deleteSearchRows(runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// SetLabel sets or clears the label on a run. Empty string clears the label.
func (s *Store) SetLabel(id int64, label string) error {
	var v any
	if label != "" {
		v = label
	}
	_, err := s.db.Exec(`UPDATE runs SET label=? WHERE id=?`, v, id)
	return err
}

// CountStaleRuns returns the number of runs still in "running" status that
// started more than threshold ago.
func (s *Store) CountStaleRuns(threshold time.Duration) (int64, error) {
	cutoff := fmtTime(time.Now().Add(-threshold))
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM runs WHERE status=? AND started_at < ?`, StatusRunning, cutoff).Scan(&n)
	return n, err
}

// MarkStaleRunsKilled sets status=killed on runs that are still in "running"
// state but started more than threshold ago. Returns the number of rows updated.
func (s *Store) MarkStaleRunsKilled(threshold time.Duration) (int64, error) {
	cutoff := fmtTime(time.Now().Add(-threshold))
	res, err := s.db.Exec(
		`UPDATE runs SET status=?, finished_at=started_at, duration_ms=0, exit_code=-1
		 WHERE status=? AND started_at < ?`,
		StatusKilled, StatusRunning, cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n > 0 {
		// Re-index affected rows so FTS reflects the new status.
		rows, err := s.db.Query(
			runSelect+` WHERE r.status=? AND r.started_at < ?`,
			StatusKilled, cutoff,
		)
		if err == nil {
			runs, _ := scanRuns(rows)
			_ = rows.Close()
			for _, r := range runs {
				_ = s.indexRun(r.ID)
			}
		}
	}
	return n, nil
}

// ListOutsideKeepLast returns runs that would be deleted by DeleteKeepLast.
func (s *Store) ListOutsideKeepLast(n int) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.id NOT IN (SELECT id FROM runs ORDER BY id DESC LIMIT ?) ORDER BY r.id DESC`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

// DeleteKeepLast deletes all runs except the n most recent (by id).
func (s *Store) DeleteKeepLast(n int) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.id NOT IN (SELECT id FROM runs ORDER BY id DESC LIMIT ?)`, n)
	if err != nil {
		return nil, err
	}
	runs, err := scanRuns(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM runs WHERE id NOT IN (SELECT id FROM runs ORDER BY id DESC LIMIT ?)`, n); err != nil {
		return nil, err
	}
	if err := s.deleteSearchRows(runs); err != nil {
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
	var label, envJSON sql.NullString
	var sessionID sql.NullInt64
	err := rows.Scan(&r.ID, &r.Status, &r.Mode, &r.Command, &r.ArgvJSON, &r.CWD, &started, &finished,
		&duration, &exitCode, &r.Hostname, &r.Shell, &r.GitRoot, &r.GitBranch, &r.GitCommit, &dirty,
		&r.StdoutPath, &r.StderrPath, &r.TerminalOutputPath, &notifyRequested, &notifyAlways, &created, &label, &envJSON, &sessionID)
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
	if label.Valid {
		r.Label = label.String
	}
	if envJSON.Valid {
		r.EnvJSON = envJSON.String
	}
	if sessionID.Valid {
		v := sessionID.Int64
		r.SessionID = &v
	}
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

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// InsertOrGetEnvironment stores env JSON (already redacted at call site) and
// returns its ID. Deduplicates by SHA-256 hash so identical envs share one row.
func (s *Store) InsertOrGetEnvironment(envJSON string) (int64, error) {
	h := sha256.Sum256([]byte(envJSON))
	hash := hex.EncodeToString(h[:])
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO environments(hash, env_json) VALUES (?,?)`, hash, envJSON); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM environments WHERE hash=?`, hash).Scan(&id)
	return id, err
}

// PruneOrphanedEnvironments deletes environment rows no longer referenced by any run.
// Call this after deleting runs to reclaim space.
func (s *Store) PruneOrphanedEnvironments() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM environments WHERE id NOT IN (SELECT env_id FROM runs WHERE env_id IS NOT NULL)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// migrateEnvData is a one-time Go-level migration that moves existing env_json
// values from the runs table into the environments dedup table and back-fills env_id.
// Values are redacted via redactor before being written to environments.
// Runs with env_json already null or with env_id already set are skipped.
func migrateEnvData(db *sql.DB, redactor logs.Redactor) error {
	rows, err := db.Query(`SELECT id, env_json FROM runs WHERE env_json IS NOT NULL AND env_id IS NULL`)
	if err != nil {
		return err
	}
	type pending struct {
		id      int64
		envJSON string
	}
	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.envJSON); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, p)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, p := range items {
		redacted, err := redactEnvJSON(p.envJSON, redactor)
		if err != nil {
			redacted = p.envJSON
		}
		h := sha256.Sum256([]byte(redacted))
		hash := hex.EncodeToString(h[:])
		if _, err := db.Exec(`INSERT OR IGNORE INTO environments(hash, env_json) VALUES (?,?)`, hash, redacted); err != nil {
			return err
		}
		var envID int64
		if err := db.QueryRow(`SELECT id FROM environments WHERE hash=?`, hash).Scan(&envID); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE runs SET env_id=?, env_json=NULL WHERE id=?`, envID, p.id); err != nil {
			return err
		}
	}
	return nil
}

// redactEnvJSON parses a JSON object of env vars, redacts each value, and re-serialises.
func redactEnvJSON(envJSON string, redactor logs.Redactor) (string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(envJSON), &m); err != nil {
		return "", err
	}
	for k, v := range m {
		m[k] = string(redactor.Redact([]byte(v)))
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Store) CreateSession(r CreateSession) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO shell_sessions (label, started_at) VALUES (?, ?)`,
		r.Label, fmtTime(r.StartedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EndSession(id int64, endedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE shell_sessions SET ended_at=? WHERE id=?`, fmtTime(endedAt), id)
	return err
}

func (s *Store) UpdateSessionLabel(id int64, label string) error {
	_, err := s.db.Exec(`UPDATE shell_sessions SET label=? WHERE id=?`, label, id)
	return err
}

func (s *Store) GetSession(id int64) (*Session, error) {
	row := s.db.QueryRow(`SELECT id, label, started_at, ended_at FROM shell_sessions WHERE id=?`, id)
	return scanSession(row)
}

func (s *Store) FindSessionByLabel(label string) (*Session, error) {
	row := s.db.QueryRow(`SELECT id, label, started_at, ended_at FROM shell_sessions WHERE label=? ORDER BY id DESC LIMIT 1`, label)
	return scanSession(row)
}

func (s *Store) GetSessionsByIDs(ids []int64) (map[int64]Session, error) {
	if len(ids) == 0 {
		return map[int64]Session{}, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT id, label, started_at, ended_at FROM shell_sessions WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[int64]Session)
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		result[sess.ID] = *sess
	}
	return result, rows.Err()
}

func (s *Store) ReopenSession(id int64) error {
	_, err := s.db.Exec(`UPDATE shell_sessions SET ended_at=NULL WHERE id=?`, id)
	return err
}

func (s *Store) ListSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(`SELECT id, label, started_at, ended_at FROM shell_sessions ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sessions []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(row sessionScanner) (*Session, error) {
	var sess Session
	var started sql.NullString
	var ended sql.NullString
	var label string
	if err := row.Scan(&sess.ID, &label, &started, &ended); err != nil {
		return nil, err
	}
	sess.Label = label
	sess.StartedAt, _ = parseTime(started.String)
	if ended.Valid && ended.String != "" {
		t, _ := parseTime(ended.String)
		sess.EndedAt = &t
	}
	return &sess, nil
}
