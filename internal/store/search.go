package store

import (
	"errors"
	"io"
	"os"
	"strings"
)

const maxSearchOutputBytes = 256 * 1024

func (s *Store) SearchRuns(query string, limit int) ([]SearchResult, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, errors.New("search query must contain text")
	}

	// Stage 1: FTS5 — exact token + prefix match, ranked by relevance.
	seen := make(map[int64]bool)
	results, _ := s.ftsSearch(query, limit, seen)

	// Stage 2: LIKE fallback — substring match on command + cwd for each term.
	// Catches cases like searching "test" matching "pytest" where FTS5 sees only whole tokens.
	if len(results) < limit {
		extra, err := s.likeSearch(terms, limit-len(results), seen)
		if err != nil {
			return nil, err
		}
		results = append(results, extra...)
	}

	return results, nil
}

// ftsSearch runs an FTS5 query and returns matched results ranked by bm25.
// seen is updated in-place so the caller can deduplicate against it.
func (s *Store) ftsSearch(query string, limit int, seen map[int64]bool) ([]SearchResult, error) {
	fts, err := ftsQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT
r.id,
snippet(run_search, -1, '', '', ' ... ', 12)
FROM run_search
JOIN runs r ON r.id = run_search.rowid
WHERE run_search MATCH ?
ORDER BY bm25(run_search), r.id DESC
LIMIT ?`, fts, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var results []SearchResult
	for rows.Next() {
		var id int64
		var snippet string
		if err := rows.Scan(&id, &snippet); err != nil {
			return nil, err
		}
		seen[id] = true
		run, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		results = append(results, SearchResult{Run: *run, Snippet: snippet})
	}
	return results, rows.Err()
}

// likeSearch does AND-of-substrings matching on command and cwd columns,
// skipping IDs already in seen.
func (s *Store) likeSearch(terms []string, limit int, seen map[int64]bool) ([]SearchResult, error) {
	// Build: WHERE (command LIKE ? OR cwd LIKE ?) AND (command LIKE ? OR cwd LIKE ?) ...
	whereClauses := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*2)
	for _, t := range terms {
		whereClauses = append(whereClauses, "(command LIKE ? OR cwd LIKE ?)")
		pattern := "%" + t + "%"
		args = append(args, pattern, pattern)
	}
	args = append(args, limit+len(seen))
	q := `SELECT id,status,mode,command,argv_json,cwd,started_at,finished_at,duration_ms,exit_code,hostname,shell,git_root,git_branch,git_commit,git_dirty,stdout_path,stderr_path,terminal_output_path,notify_requested,notify_always,created_at,label,env_json,session_id
FROM runs WHERE ` + strings.Join(whereClauses, " AND ") + ` ORDER BY id DESC LIMIT ?`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var results []SearchResult
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		if seen[run.ID] {
			continue
		}
		seen[run.ID] = true
		if len(results) >= limit {
			break
		}
		results = append(results, SearchResult{Run: *run})
	}
	return results, rows.Err()
}

func (s *Store) indexRun(id int64) error {
	r, err := s.GetRun(id)
	if err != nil {
		return err
	}
	return s.upsertSearch(id, r.Command, r.CWD, combinedOutput(r))
}

func (s *Store) upsertSearch(id int64, command, cwd, output string) error {
	if _, err := s.db.Exec(`DELETE FROM run_search WHERE rowid=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO run_search(rowid, command, cwd, output) VALUES (?, ?, ?, ?)`, id, command, cwd, output)
	return err
}

func (s *Store) deleteSearchRows(runs []Run) error {
	for _, r := range runs {
		if _, err := s.db.Exec(`DELETE FROM run_search WHERE rowid=?`, r.ID); err != nil {
			return err
		}
	}
	return nil
}

func ftsQuery(query string) (string, error) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "", errors.New("search query must contain text")
	}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		terms = append(terms, `"`+strings.ReplaceAll(field, `"`, `""`)+`"`)
	}
	return strings.Join(terms, " "), nil
}

func combinedOutput(r *Run) string {
	var b strings.Builder
	for _, path := range []string{r.StdoutPath, r.StderrPath, r.TerminalOutputPath} {
		text := readLimitedText(path, maxSearchOutputBytes-int64(b.Len()))
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
		if int64(b.Len()) >= maxSearchOutputBytes {
			break
		}
	}
	return b.String()
}

func readLimitedText(path string, remaining int64) string {
	if path == "" || remaining <= 0 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, remaining))
	if err != nil {
		return ""
	}
	return string(b)
}
