package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSearchRunsIndexesCommandCWDAndOutput(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "go test ./...",
		ArgvJSON:  `["go","test","./..."]`,
		CWD:       "/tmp/search-project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	logDir := t.TempDir()
	stdoutPath := filepath.Join(logDir, "stdout.log")
	stderrPath := filepath.Join(logDir, "stderr.log")
	if err := os.WriteFile(stdoutPath, []byte("tests passed\nunique-output-token\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("warning stream"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	assertSearchHit(t, st, "go test", id)
	assertSearchHit(t, st, "search-project", id)
	results := assertSearchHit(t, st, "unique-output-token", id)
	if results[0].Snippet == "" {
		t.Fatalf("expected search snippet: %#v", results[0])
	}
}

func TestSearchRunsHandlesSpecialCharacters(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   `bash -c "echo [value]"`,
		ArgvJSON:  `["bash","-c","echo [value]"]`,
		CWD:       "/tmp/project",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	assertSearchHit(t, st, `"echo`, id)
}

func TestDeleteOlderThanRemovesSearchRows(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	oldID, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "old searchable-command",
		ArgvJSON:  `["old"]`,
		CWD:       "/tmp",
		StartedAt: time.Now().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}
	if _, err := st.DeleteOlderThan(time.Now().Add(-24 * time.Hour)); err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	results, err := st.SearchRuns("searchable-command", 10)
	if err != nil {
		t.Fatalf("search runs: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted run %d still searchable: %#v", oldID, results)
	}
}

func assertSearchHit(t *testing.T, st *Store, query string, id int64) []SearchResult {
	t.Helper()
	results, err := st.SearchRuns(query, 10)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if len(results) == 0 || results[0].Run.ID != id {
		t.Fatalf("search %q results mismatch: %#v", query, results)
	}
	return results
}
