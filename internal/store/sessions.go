package store

import (
	"database/sql"
	"time"
)

// SetSessionKept pins (or unpins) a session so it never expires. It returns
// sql.ErrNoRows when the session does not exist.
func (s *Store) SetSessionKept(id int64, kept bool) error {
	res, err := s.db.Exec(`UPDATE shell_sessions SET kept=? WHERE id=?`, boolInt(kept), id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SessionIsEmpty reports whether a session has no label, runs, or notes, so
// nothing would be lost by deleting it.
func (s *Store) SessionIsEmpty(id int64) (bool, error) {
	var empty bool
	err := s.db.QueryRow(`SELECT label = ''
	AND NOT EXISTS (SELECT 1 FROM runs WHERE session_id = shell_sessions.id)
	AND NOT EXISTS (SELECT 1 FROM notes WHERE session_id = shell_sessions.id)
FROM shell_sessions WHERE id=?`, id).Scan(&empty)
	return empty, err
}

// ExpiredAutoSessions returns automatically started sessions that ended
// before cutoff and are not worth keeping: not pinned, no label, no notes.
// Sessions still open (or re-opened by attach) never expire.
func (s *Store) ExpiredAutoSessions(cutoff time.Time) ([]Session, error) {
	rows, err := s.db.Query(`SELECT `+sessionCols+` FROM shell_sessions
WHERE auto = 1 AND kept = 0 AND label = '' AND ended_at IS NOT NULL AND ended_at < ?
	AND NOT EXISTS (SELECT 1 FROM notes WHERE notes.session_id = shell_sessions.id)
ORDER BY id`, fmtTime(cutoff))
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

// SessionRuns returns the runs recorded in a session, oldest first.
func (s *Store) SessionRuns(id int64) ([]Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.session_id=? ORDER BY r.id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

// DeleteSession removes a session with its runs, their search entries, and
// its notes. It returns the deleted runs so the caller can remove their logs.
func (s *Store) DeleteSession(id int64) ([]Run, error) {
	runs, err := s.SessionRuns(id)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM notes WHERE session_id=? OR run_id IN (SELECT id FROM runs WHERE session_id=?)`, []any{id, id}},
		{`DELETE FROM run_search WHERE rowid IN (SELECT id FROM runs WHERE session_id=?)`, []any{id}},
		{`DELETE FROM runs WHERE session_id=?`, []any{id}},
		{`DELETE FROM session_branches WHERE session_id=?`, []any{id}},
		{`DELETE FROM shell_sessions WHERE id=?`, []any{id}},
	} {
		if _, err := tx.Exec(stmt.query, stmt.args...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return runs, nil
}
