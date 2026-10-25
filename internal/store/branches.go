package store

import (
	"database/sql"
	"errors"
	"time"
)

// BranchLink links a Git branch in one repository to the session for its
// work, so carrier can point a shell that checks the branch out at it.
type BranchLink struct {
	GitRoot   string
	Branch    string
	SessionID int64
	LinkedAt  time.Time
}

// LinkBranch links branch in the repository at root to a session, replacing
// any session it was linked to. root should be canonical, see
// cli.canonicalRepoRoot, so every spelling of the repository matches.
func (s *Store) LinkBranch(root, branch string, sessionID int64, at time.Time) error {
	_, err := s.db.Exec(`INSERT INTO session_branches (git_root, branch, session_id, linked_at) VALUES (?, ?, ?, ?)
ON CONFLICT (git_root, branch) DO UPDATE SET session_id=excluded.session_id, linked_at=excluded.linked_at`,
		root, branch, sessionID, fmtTime(at))
	return err
}

// BranchSession returns the session branch in the repository at root is
// linked to, or nil when there is none.
func (s *Store) BranchSession(root, branch string) (*Session, error) {
	row := s.db.QueryRow(`SELECT `+sessionColsQualified+` FROM session_branches b
JOIN shell_sessions s ON s.id = b.session_id WHERE b.git_root = ? AND b.branch = ?`, root, branch)
	sess, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return sess, err
}

// SessionBranches returns the branches linked to a session, oldest link
// first.
func (s *Store) SessionBranches(sessionID int64) ([]BranchLink, error) {
	rows, err := s.db.Query(`SELECT git_root, branch, session_id, linked_at FROM session_branches
WHERE session_id = ? ORDER BY linked_at, branch`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var links []BranchLink
	for rows.Next() {
		var l BranchLink
		var at string
		if err := rows.Scan(&l.GitRoot, &l.Branch, &l.SessionID, &at); err != nil {
			return nil, err
		}
		l.LinkedAt, _ = parseTime(at)
		links = append(links, l)
	}
	return links, rows.Err()
}

// UnlinkBranch removes the link of branch in the repository at root. It
// reports whether there was one.
func (s *Store) UnlinkBranch(root, branch string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM session_branches WHERE git_root = ? AND branch = ?`, root, branch)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
