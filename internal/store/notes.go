package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CreateNote holds the fields for a new note. At least one of SessionID or
// RunID must be set.
type CreateNote struct {
	SessionID *int64
	RunID     *int64
	Text      string
	CreatedAt time.Time
	Kind      string
}

// NoteFilter restricts ListNotes. Zero-value fields are ignored; when both
// SessionIDs and RunIDs are set, notes matching either are returned.
type NoteFilter struct {
	SessionIDs []int64
	RunIDs     []int64
}

const noteSelect = `SELECT id, session_id, run_id, text, created_at, kind FROM notes`

func (s *Store) CreateNote(n CreateNote) (int64, error) {
	if strings.TrimSpace(n.Text) == "" {
		return 0, errors.New("note text must not be empty")
	}
	if n.SessionID == nil && n.RunID == nil {
		return 0, errors.New("note must belong to a session or a run")
	}
	res, err := s.db.Exec(`INSERT INTO notes (session_id, run_id, text, created_at, kind) VALUES (?, ?, ?, ?, ?)`,
		nullableInt64(n.SessionID), nullableInt64(n.RunID), n.Text, fmtTime(n.CreatedAt), n.Kind)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if n.RunID != nil {
		if err := s.indexRun(*n.RunID); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (s *Store) GetNote(id int64) (*Note, error) {
	return scanNote(s.db.QueryRow(noteSelect+` WHERE id=?`, id))
}

// DeleteNote removes a note and refreshes the search index of the run it was
// attached to. Returns sql.ErrNoRows if no note with the given id exists.
func (s *Store) DeleteNote(id int64) (*Note, error) {
	n, err := s.GetNote(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM notes WHERE id=?`, id); err != nil {
		return nil, err
	}
	if n.RunID != nil {
		if err := s.indexRun(*n.RunID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return n, nil
}

// ListNotes returns notes oldest-first, optionally restricted by f.
func (s *Store) ListNotes(f NoteFilter, limit int) ([]Note, error) {
	var clauses []string
	var args []any
	if len(f.SessionIDs) > 0 {
		clauses = append(clauses, "session_id IN ("+placeholders(len(f.SessionIDs))+")")
		for _, id := range f.SessionIDs {
			args = append(args, id)
		}
	}
	if len(f.RunIDs) > 0 {
		clauses = append(clauses, "run_id IN ("+placeholders(len(f.RunIDs))+")")
		for _, id := range f.RunIDs {
			args = append(args, id)
		}
	}
	q := noteSelect
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " OR ")
	}
	// Take the newest `limit` notes, then return them oldest-first.
	q = `SELECT * FROM (` + q + ` ORDER BY id DESC LIMIT ?) ORDER BY id`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var notes []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, *n)
	}
	return notes, rows.Err()
}

// NotesBetween returns the notes written from from up to, not including, to,
// oldest first.
func (s *Store) NotesBetween(from, to time.Time) ([]Note, error) {
	rows, err := s.db.Query(noteSelect+` WHERE created_at >= ? AND created_at < ? ORDER BY created_at, id`, fmtTime(from), fmtTime(to))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var notes []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, *n)
	}
	return notes, rows.Err()
}

// SessionOutcome returns the latest outcome note of a session, or nil when it
// has none.
func (s *Store) SessionOutcome(sessionID int64) (*Note, error) {
	n, err := scanNote(s.db.QueryRow(noteSelect+` WHERE session_id=? AND kind=? ORDER BY id DESC LIMIT 1`, sessionID, NoteOutcome))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return n, err
}

// LatestSessionRun returns the most recent run recorded in a session.
func (s *Store) LatestSessionRun(sessionID int64) (*Run, error) {
	rows, err := s.db.Query(runSelect+` WHERE r.session_id=? ORDER BY r.id DESC LIMIT 1`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRun(rows)
}

// PruneOrphanedNotes handles notes whose run has been deleted. Notes that
// still belong to a session are detached from the run and kept; notes with
// no session are deleted. Returns the number of notes deleted.
func (s *Store) PruneOrphanedNotes() (int64, error) {
	if _, err := s.db.Exec(`UPDATE notes SET run_id=NULL
WHERE run_id IS NOT NULL AND session_id IS NOT NULL AND run_id NOT IN (SELECT id FROM runs)`); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM notes
WHERE session_id IS NULL AND (run_id IS NULL OR run_id NOT IN (SELECT id FROM runs))`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// runNotesText returns the text of all notes attached to a run, one per line,
// for inclusion in the search index.
func (s *Store) runNotesText(runID int64) (string, error) {
	rows, err := s.db.Query(`SELECT text FROM notes WHERE run_id=? ORDER BY id`, runID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var texts []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return "", err
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n"), rows.Err()
}

func scanNote(row sessionScanner) (*Note, error) {
	var n Note
	var sessionID, runID sql.NullInt64
	var created string
	if err := row.Scan(&n.ID, &sessionID, &runID, &n.Text, &created, &n.Kind); err != nil {
		return nil, err
	}
	if sessionID.Valid {
		v := sessionID.Int64
		n.SessionID = &v
	}
	if runID.Valid {
		v := runID.Int64
		n.RunID = &v
	}
	n.CreatedAt, _ = parseTime(created)
	return &n, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
