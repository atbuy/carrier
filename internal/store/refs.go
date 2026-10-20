package store

import "database/sql"

// NthLatestRun returns the n-th most recent run: 1 is the latest, 2 the one
// before it. It returns sql.ErrNoRows when fewer than n runs exist.
func (s *Store) NthLatestRun(n int) (*Run, error) {
	if n < 1 {
		return nil, sql.ErrNoRows
	}
	rows, err := s.db.Query(runSelect+` ORDER BY r.id DESC LIMIT 1 OFFSET ?`, n-1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRun(rows)
}

// ListLabeledRuns returns every run with a label, newest first.
func (s *Store) ListLabeledRuns() ([]Run, error) {
	rows, err := s.db.Query(runSelect + ` WHERE COALESCE(r.label, '') != '' ORDER BY r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRuns(rows)
}

// ListLabeledSessions returns every session with a label, newest first.
func (s *Store) ListLabeledSessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT ` + sessionCols + ` FROM shell_sessions WHERE label != '' ORDER BY id DESC`)
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
