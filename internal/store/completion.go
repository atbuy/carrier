package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenReadOnly opens an existing store for reading, for shell completion. It
// never creates the database or runs migrations, and gives up quickly when
// the database is locked, so a Tab press cannot stall the shell or change
// anything on disk.
func OpenReadOnly(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "carrier.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "mode=ro&_pragma=busy_timeout(200)",
	}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

// RunChoice is a run offered by shell completion.
type RunChoice struct {
	ID        int64
	Status    string
	Command   string
	Label     string
	StartedAt time.Time
}

// RunChoiceFilter narrows the runs offered by completion.
type RunChoiceFilter struct {
	// IDPrefix keeps runs whose ID starts with these digits.
	IDPrefix string
	// LabelPrefix keeps labeled runs whose label starts with this text,
	// ignoring case.
	LabelPrefix string
	// Running keeps only runs that are still running.
	Running bool
	// Exclude leaves out these run IDs.
	Exclude []int64
	Limit   int
}

// RunChoices returns the newest runs matching f, newest first.
func (s *Store) RunChoices(f RunChoiceFilter) ([]RunChoice, error) {
	where, args := []string{"1=1"}, []any{}
	if f.IDPrefix != "" {
		where = append(where, "CAST(id AS TEXT) LIKE ? ESCAPE '\\'")
		args = append(args, likePrefix(f.IDPrefix))
	}
	if f.LabelPrefix != "" {
		where = append(where, "label LIKE ? ESCAPE '\\'")
		args = append(args, likePrefix(f.LabelPrefix))
	}
	if f.Running {
		where = append(where, "status = ?")
		args = append(args, StatusRunning)
	}
	for _, id := range f.Exclude {
		where = append(where, "id != ?")
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT id, status, command, COALESCE(label, ''), started_at FROM runs
WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, append(args, limitOr(f.Limit, 20))...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RunChoice
	for rows.Next() {
		var c RunChoice
		var started string
		if err := rows.Scan(&c.ID, &c.Status, &c.Command, &c.Label, &started); err != nil {
			return nil, err
		}
		c.StartedAt, _ = parseTime(started)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SessionChoice is a session offered by shell completion.
type SessionChoice struct {
	ID        int64
	Label     string
	StartedAt time.Time
	Ended     bool
	Kept      bool
	Runs      int
}

// SessionChoiceFilter narrows the sessions offered by completion.
type SessionChoiceFilter struct {
	IDPrefix    string
	LabelPrefix string
	// Kept keeps only kept sessions when true and only unkept ones when false.
	Kept    *bool
	Exclude []int64
	Limit   int
}

// SessionChoices returns the newest sessions matching f, newest first.
// Automatic sessions with no runs and no label are left out: there is
// nothing in them to attach to or look at.
func (s *Store) SessionChoices(f SessionChoiceFilter) ([]SessionChoice, error) {
	where := []string{"(s.label != '' OR s.auto = 0 OR EXISTS (SELECT 1 FROM runs r WHERE r.session_id = s.id))"}
	var args []any
	if f.IDPrefix != "" {
		where = append(where, "CAST(s.id AS TEXT) LIKE ? ESCAPE '\\'")
		args = append(args, likePrefix(f.IDPrefix))
	}
	if f.LabelPrefix != "" {
		where = append(where, "s.label LIKE ? ESCAPE '\\'")
		args = append(args, likePrefix(f.LabelPrefix))
	}
	if f.Kept != nil {
		where = append(where, "s.kept = ?")
		args = append(args, boolInt(*f.Kept))
	}
	for _, id := range f.Exclude {
		where = append(where, "s.id != ?")
		args = append(args, id)
	}
	rows, err := s.db.Query(`SELECT s.id, s.label, s.started_at, COALESCE(s.ended_at, ''), s.kept,
  (SELECT COUNT(*) FROM runs r WHERE r.session_id = s.id)
FROM shell_sessions s WHERE `+strings.Join(where, " AND ")+` ORDER BY s.id DESC LIMIT ?`, append(args, limitOr(f.Limit, 20))...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []SessionChoice
	for rows.Next() {
		var c SessionChoice
		var started, ended string
		var kept int
		if err := rows.Scan(&c.ID, &c.Label, &started, &ended, &kept, &c.Runs); err != nil {
			return nil, err
		}
		c.StartedAt, _ = parseTime(started)
		c.Ended, c.Kept = ended != "", kept != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// RunValueColumn names a runs column whose values completion offers.
type RunValueColumn string

const (
	ValueBranch  RunValueColumn = "git_branch"
	ValueLabel   RunValueColumn = "label"
	ValueCWD     RunValueColumn = "cwd"
	ValueCommand RunValueColumn = "command"
)

// RunValues returns the distinct non-empty values of col that start with
// prefix: the most used first for commands, the most recently used first
// for everything else.
func (s *Store) RunValues(col RunValueColumn, prefix string, limit int) ([]string, error) {
	switch col {
	case ValueBranch, ValueLabel, ValueCWD, ValueCommand:
	default:
		return nil, fmt.Errorf("unknown column %q", col)
	}
	order := "MAX(id) DESC"
	if col == ValueCommand {
		order = "COUNT(*) DESC, MAX(id) DESC"
	}
	c := string(col)
	rows, err := s.db.Query(`SELECT `+c+` FROM runs WHERE `+c+` != '' AND `+c+` LIKE ? ESCAPE '\'
GROUP BY `+c+` ORDER BY `+order+` LIMIT ?`, likePrefix(prefix), limitOr(limit, 20))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// NoteChoice is a note offered by shell completion.
type NoteChoice struct {
	ID   int64
	Text string
}

// NoteChoices returns the newest notes whose ID starts with idPrefix.
func (s *Store) NoteChoices(idPrefix string, limit int) ([]NoteChoice, error) {
	rows, err := s.db.Query(`SELECT id, text FROM notes WHERE CAST(id AS TEXT) LIKE ? ESCAPE '\'
ORDER BY id DESC LIMIT ?`, likePrefix(idPrefix), limitOr(limit, 20))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []NoteChoice
	for rows.Next() {
		var c NoteChoice
		if err := rows.Scan(&c.ID, &c.Text); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// likePrefix escapes s for LIKE and matches anything that starts with it.
func likePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s) + "%"
}

func limitOr(limit, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	return limit
}
