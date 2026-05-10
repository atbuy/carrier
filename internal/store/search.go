package store

import (
	"errors"
	"io"
	"os"
	"strings"
)

const maxSearchOutputBytes = 256 * 1024

func (s *Store) SearchRuns(query string, limit int) ([]SearchResult, error) {
	ftsQuery, err := ftsQuery(query)
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
LIMIT ?`, ftsQuery, limit)
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
		run, err := s.GetRun(id)
		if err != nil {
			return nil, err
		}
		results = append(results, SearchResult{Run: *run, Snippet: snippet})
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
