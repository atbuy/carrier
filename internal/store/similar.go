package store

import (
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	// signatureTailBytes is how much of the end of a run's output is scanned
	// for error lines; errors are almost always near the end.
	signatureTailBytes = 64 * 1024
	// signatureMaxLines and signatureMaxTerms bound the search query.
	signatureMaxLines = 3
	signatureMaxTerms = 8
	// signatureMinTerms avoids matching on one or two common words.
	signatureMinTerms = 2
)

var (
	errorLinePattern = regexp.MustCompile(`(?i)error|fail|exception|fatal|panic|refused|denied|not found|no such|timed? ?out|cannot|unable|invalid|traceback|segmentation`)
	signatureWord    = regexp.MustCompile(`[A-Za-z][A-Za-z_.-]*[A-Za-z]`)
	signatureStop    = map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true, "this": true,
		"that": true, "was": true, "are": true, "not": true, "has": true, "have": true,
		"you": true, "your": true, "into": true, "but": true, "all": true, "any": true,
	}
)

// SimilarRun is a past failed run whose output shares the error signature of
// another run.
type SimilarRun struct {
	Run       Run
	NoteCount int
}

// SimilarFailures returns failed runs other than runID that started before
// `before` and whose output contains the error signature of runID's output.
// Runs with notes or a label come first, since they explain what happened,
// then the closest matches. It returns no results when runID's output has no
// usable signature.
func (s *Store) SimilarFailures(runID int64, before time.Time, limit int) ([]SimilarRun, error) {
	r, err := s.GetRun(runID)
	if err != nil {
		return nil, err
	}
	terms := ErrorSignature(runOutputTail(r))
	if len(terms) < signatureMinTerms {
		return nil, nil
	}
	// Match only the output column (which also holds note text), not the
	// command or working directory.
	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	rows, err := s.db.Query(`SELECT r.id, (SELECT COUNT(*) FROM notes n WHERE n.run_id = r.id) AS note_count
FROM run_search
JOIN runs r ON r.id = run_search.rowid
WHERE run_search MATCH ? AND r.status = ? AND r.id != ? AND r.started_at < ?
ORDER BY (note_count > 0 OR COALESCE(r.label, '') != '') DESC, bm25(run_search), r.id DESC
LIMIT ?`, "output : ("+strings.Join(quoted, " ")+")", StatusFailed, runID, fmtTime(before), limit)
	if err != nil {
		return nil, err
	}
	type match struct {
		id    int64
		notes int
	}
	var matches []match
	for rows.Next() {
		var m match
		if err := rows.Scan(&m.id, &m.notes); err != nil {
			_ = rows.Close()
			return nil, err
		}
		matches = append(matches, m)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]SimilarRun, 0, len(matches))
	for _, m := range matches {
		run, err := s.GetRun(m.id)
		if err != nil {
			return nil, err
		}
		results = append(results, SimilarRun{Run: *run, NoteCount: m.notes})
	}
	return results, nil
}

// ErrorSignature extracts search terms describing the error in output: words
// from the last few lines that look like errors (or the last non-empty line
// when none do). Words containing digits are dropped, so ports, PIDs, line
// numbers, and hashes do not prevent a match.
func ErrorSignature(output string) []string {
	lines := strings.Split(ansi.Strip(output), "\n")
	var picked []string
	for i := len(lines) - 1; i >= 0 && len(picked) < signatureMaxLines; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" && errorLinePattern.MatchString(line) {
			picked = append(picked, line)
		}
	}
	if len(picked) == 0 {
		for i := len(lines) - 1; i >= 0; i-- {
			if line := strings.TrimSpace(lines[i]); line != "" {
				picked = append(picked, line)
				break
			}
		}
	}
	seen := map[string]bool{}
	var terms []string
	// picked is newest-first; the last error line is usually the most specific.
	for _, line := range picked {
		for _, field := range strings.Fields(line) {
			if strings.ContainsAny(field, "0123456789") {
				continue
			}
			for _, word := range signatureWord.FindAllString(field, -1) {
				word = strings.ToLower(word)
				if len(word) < 3 || signatureStop[word] || seen[word] {
					continue
				}
				seen[word] = true
				terms = append(terms, word)
				if len(terms) == signatureMaxTerms {
					return terms
				}
			}
		}
	}
	return terms
}

// runOutputTail returns the end of a run's error-bearing output: stderr when
// it has content, otherwise the terminal log (shell runs) or stdout.
func runOutputTail(r *Run) string {
	for _, path := range []string{r.StderrPath, r.TerminalOutputPath, r.StdoutPath} {
		if text := readTail(path, signatureTailBytes); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func readTail(path string, n int64) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err == nil && info.Size() > n {
		if _, err := f.Seek(-n, io.SeekEnd); err != nil {
			return ""
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(b)
}
