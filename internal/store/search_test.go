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
	results, err := st.SearchRuns("searchable-command", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("search runs: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted run %d still searchable: %#v", oldID, results)
	}
}

// ---------------------------------------------------------------------------
// SearchRuns edge cases
// ---------------------------------------------------------------------------

func TestSearchRunsEmptyQuery(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, err = st.SearchRuns("", 10, SearchFilter{})
	if err == nil {
		t.Fatal("SearchRuns with empty query: expected error, got nil")
	}
}

func TestSearchRunsWhitespaceOnlyQuery(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	_, err = st.SearchRuns("   ", 10, SearchFilter{})
	if err == nil {
		t.Fatal("SearchRuns with whitespace query: expected error, got nil")
	}
}

func TestSearchRunsNoResults(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "make build",
		ArgvJSON:  `["make","build"]`,
		CWD:       "/tmp/proj",
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	results, err := st.SearchRuns("nonexistentxyzterm", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchRuns: want 0 results, got %d", len(results))
	}
}

func TestSearchRunsFilterSince(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	oldID, err := st.CreateRun(CreateRun{
		Status: StatusSuccess, Mode: ModeRun, Command: "filtertest old",
		ArgvJSON: `["filtertest","old"]`, CWD: "/tmp", StartedAt: old,
	})
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}
	recentID, err := st.CreateRun(CreateRun{
		Status: StatusSuccess, Mode: ModeRun, Command: "filtertest recent",
		ArgvJSON: `["filtertest","recent"]`, CWD: "/tmp", StartedAt: recent,
	})
	if err != nil {
		t.Fatalf("create recent run: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	results, err := st.SearchRuns("filtertest", 10, SearchFilter{Since: &cutoff})
	if err != nil {
		t.Fatalf("SearchRuns with Since: %v", err)
	}
	ids := make(map[int64]bool)
	for _, r := range results {
		ids[r.Run.ID] = true
	}
	if ids[oldID] {
		t.Errorf("old run should be excluded by --since filter")
	}
	if !ids[recentID] {
		t.Errorf("recent run should be included by --since filter")
	}
}

func TestSearchRunsFilterStatus(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	successID, err := st.CreateRun(CreateRun{
		Status: StatusSuccess, Mode: ModeRun, Command: "statusfilter cmd",
		ArgvJSON: `["statusfilter","cmd"]`, CWD: "/tmp", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create success run: %v", err)
	}
	failedID, err := st.CreateRun(CreateRun{
		Status: StatusFailed, Mode: ModeRun, Command: "statusfilter cmd",
		ArgvJSON: `["statusfilter","cmd"]`, CWD: "/tmp", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create failed run: %v", err)
	}

	results, err := st.SearchRuns("statusfilter", 10, SearchFilter{Status: StatusFailed})
	if err != nil {
		t.Fatalf("SearchRuns with Status: %v", err)
	}
	ids := make(map[int64]bool)
	for _, r := range results {
		ids[r.Run.ID] = true
	}
	if ids[successID] {
		t.Errorf("success run should be excluded by --status failed filter")
	}
	if !ids[failedID] {
		t.Errorf("failed run should be included by --status failed filter")
	}
}

func TestSearchRunsLikeSubstringFallback(t *testing.T) {
	// FTS5 tokenises on word boundaries, so "pytest" won't match the token "test"
	// via FTS. The LIKE fallback must catch it.
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "pytest -v",
		ArgvJSON:  `["pytest","-v"]`,
		CWD:       "/home/user/repo",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	assertSearchHit(t, st, "test", id)
}

func TestSearchRunsMultiTermAND(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// run A matches both terms
	idA, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "cargo build --release",
		ArgvJSON:  `["cargo","build","--release"]`,
		CWD:       "/home/user/rust-proj",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run A: %v", err)
	}

	// run B matches only one term
	if _, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "cargo check",
		ArgvJSON:  `["cargo","check"]`,
		CWD:       "/home/user/rust-proj",
		StartedAt: time.Now().Add(time.Second),
	}); err != nil {
		t.Fatalf("create run B: %v", err)
	}

	// searching for "cargo release" should only hit run A (both words present)
	results, err := st.SearchRuns("cargo release", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Run.ID == idA {
			found = true
		}
	}
	if !found {
		t.Fatalf("SearchRuns multi-term: expected run A (%d) in results %v", idA, results)
	}
}

func TestSearchRunsLimitRespected(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if _, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "limitcmd run",
			ArgvJSON:  `["limitcmd","run"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
	}

	results, err := st.SearchRuns("limitcmd", 3, SearchFilter{})
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	if len(results) > 3 {
		t.Fatalf("SearchRuns: limit=3 but got %d results", len(results))
	}
}

func TestSearchRunsDeleteKeepLastRemovesSearchRows(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	var createdIDs []int64
	for i := 0; i < 4; i++ {
		id, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "searchable-keep-cmd",
			ArgvJSON:  `["searchable-keep-cmd"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		createdIDs = append(createdIDs, id)
	}

	// keep only 2 newest; oldest 2 should disappear from search
	if _, err := st.DeleteKeepLast(2); err != nil {
		t.Fatalf("DeleteKeepLast: %v", err)
	}

	results, err := st.SearchRuns("searchable-keep-cmd", 10, SearchFilter{})
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("SearchRuns after DeleteKeepLast: want 2, got %d", len(results))
	}
	for _, r := range results {
		if r.Run.ID == createdIDs[0] || r.Run.ID == createdIDs[1] {
			t.Fatalf("SearchRuns after DeleteKeepLast: deleted run %d still found", r.Run.ID)
		}
	}
}

func TestSearchHelpersReturnErrorsAfterClose(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	seen := map[int64]bool{}
	if _, err := st.SearchRuns("closed", 10, SearchFilter{}); err == nil {
		t.Fatal("expected SearchRuns error after close")
	}
	if _, err := st.ftsSearch("closed", 10, SearchFilter{}, seen); err == nil {
		t.Fatal("expected ftsSearch error after close")
	}
	if _, err := st.likeSearch([]string{"closed"}, 10, SearchFilter{}, seen); err == nil {
		t.Fatal("expected likeSearch error after close")
	}
	if err := st.indexRun(1); err == nil {
		t.Fatal("expected indexRun error after close")
	}
	if err := st.upsertSearch(1, "cmd", "/tmp", "out"); err == nil {
		t.Fatal("expected upsertSearch error after close")
	}
	if err := st.deleteSearchRows([]Run{{ID: 1}}); err == nil {
		t.Fatal("expected deleteSearchRows error after close")
	}
}

// ---------------------------------------------------------------------------

func assertSearchHit(t *testing.T, st *Store, query string, id int64) []SearchResult {
	t.Helper()
	results, err := st.SearchRuns(query, 10, SearchFilter{})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if len(results) == 0 || results[0].Run.ID != id {
		t.Fatalf("search %q results mismatch: %#v", query, results)
	}
	return results
}
