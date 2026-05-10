package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRunLifecycle(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Now().Add(-2 * time.Second).UTC()
	dirty := true
	id, err := st.CreateRun(CreateRun{
		Status:          StatusRunning,
		Mode:            ModeRun,
		Command:         "go test ./...",
		ArgvJSON:        `["go","test","./..."]`,
		CWD:             "/tmp/project",
		StartedAt:       started,
		Hostname:        "host",
		Shell:           "/bin/bash",
		GitRoot:         "/tmp/project",
		GitBranch:       "main",
		GitCommit:       "abcdef",
		GitDirty:        &dirty,
		NotifyRequested: true,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	stdoutPath := filepath.Join(t.TempDir(), "stdout.log")
	stderrPath := filepath.Join(t.TempDir(), "stderr.log")
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, StatusFailed, 7, started.Add(1500*time.Millisecond)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	run, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("status mismatch: got %q", run.Status)
	}
	if run.ExitCode == nil || *run.ExitCode != 7 {
		t.Fatalf("exit code mismatch: got %#v", run.ExitCode)
	}
	if run.DurationMS == nil || *run.DurationMS != 1500 {
		t.Fatalf("duration mismatch: got %#v", run.DurationMS)
	}
	if run.StdoutPath != stdoutPath || run.StderrPath != stderrPath {
		t.Fatalf("paths mismatch: stdout=%q stderr=%q", run.StdoutPath, run.StderrPath)
	}
	if run.GitDirty == nil || !*run.GitDirty {
		t.Fatalf("git dirty not preserved: %#v", run.GitDirty)
	}

	latest, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID != id {
		t.Fatalf("latest id mismatch: got %d want %d", latest.ID, id)
	}

	failed, err := st.ListByStatus(StatusFailed, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(failed) != 1 || failed[0].ID != id {
		t.Fatalf("failed list mismatch: %#v", failed)
	}
}

func TestOpenAppliesGooseMigrations(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	var versionID int64
	var isApplied bool
	if err := st.db.QueryRow(`SELECT version_id, is_applied FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&versionID, &isApplied); err != nil {
		t.Fatalf("query goose version: %v", err)
	}
	if versionID != 4 || !isApplied {
		t.Fatalf("goose version mismatch: version=%d applied=%v", versionID, isApplied)
	}
	version, err := st.MigrationVersion()
	if err != nil {
		t.Fatalf("migration version: %v", err)
	}
	if version != 4 {
		t.Fatalf("migration version = %d", version)
	}

	if reopened, err := Open(dir); err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	} else {
		_ = reopened.Close()
	}
}

func TestOpenReturnsErrorWhenDataDirIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "carrier-data")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := Open(filePath); err == nil {
		t.Fatal("expected Open error when data dir is a file")
	}
}

func TestStoreMethodsReturnErrorsAfterClose(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	started := time.Now().UTC()
	if _, err := st.MigrationVersion(); err == nil {
		t.Fatal("expected MigrationVersion error after close")
	}
	if _, err := st.CreateRun(CreateRun{Status: StatusRunning, Mode: ModeRun, Command: "cmd", ArgvJSON: `["cmd"]`, CWD: "/tmp", StartedAt: started}); err == nil {
		t.Fatal("expected CreateRun error after close")
	}
	if err := st.UpdatePaths(1, "out", "err", "term"); err == nil {
		t.Fatal("expected UpdatePaths error after close")
	}
	if err := st.FinishRun(1, StatusSuccess, 0, started); err == nil {
		t.Fatal("expected FinishRun error after close")
	}
	if _, err := st.GetRun(1); err == nil {
		t.Fatal("expected GetRun error after close")
	}
	if _, err := st.Latest(); err == nil {
		t.Fatal("expected Latest error after close")
	}
	if _, err := st.ListByStatus(StatusRunning, 10); err == nil {
		t.Fatal("expected ListByStatus error after close")
	}
	if _, err := st.All(10); err == nil {
		t.Fatal("expected All error after close")
	}
	if _, err := st.AllRuns(); err == nil {
		t.Fatal("expected AllRuns error after close")
	}
	if _, err := st.ListHistory(10, HistoryFilter{Status: StatusRunning}); err == nil {
		t.Fatal("expected ListHistory error after close")
	}
	if _, err := st.ListOlderThan(started); err == nil {
		t.Fatal("expected ListOlderThan error after close")
	}
	if _, err := st.DeleteOlderThan(started); err == nil {
		t.Fatal("expected DeleteOlderThan error after close")
	}
	if err := st.SetLabel(1, "ci"); err == nil {
		t.Fatal("expected SetLabel error after close")
	}
	if _, err := st.CountStaleRuns(time.Hour); err == nil {
		t.Fatal("expected CountStaleRuns error after close")
	}
	if _, err := st.MarkStaleRunsKilled(time.Hour); err == nil {
		t.Fatal("expected MarkStaleRunsKilled error after close")
	}
	if _, err := st.ListOutsideKeepLast(10); err == nil {
		t.Fatal("expected ListOutsideKeepLast error after close")
	}
	if _, err := st.DeleteKeepLast(10); err == nil {
		t.Fatal("expected DeleteKeepLast error after close")
	}
}

func TestDeleteOlderThan(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	oldID, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "old",
		ArgvJSON:  `["old"]`,
		CWD:       "/tmp",
		StartedAt: time.Now().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}
	newID, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "new",
		ArgvJSON:  `["new"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create new run: %v", err)
	}

	deleted, err := st.DeleteOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	if len(deleted) != 1 || deleted[0].ID != oldID {
		t.Fatalf("deleted mismatch: %#v", deleted)
	}
	if _, err := st.GetRun(oldID); err == nil {
		t.Fatalf("old run still exists")
	}
	if _, err := st.GetRun(newID); err != nil {
		t.Fatalf("new run missing: %v", err)
	}
}

func TestListOlderThanDoesNotDelete(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	oldID, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "old",
		ArgvJSON:  `["old"]`,
		CWD:       "/tmp",
		StartedAt: time.Now().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}
	if _, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "new",
		ArgvJSON:  `["new"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create new run: %v", err)
	}

	runs, err := st.ListOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("list older than: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != oldID {
		t.Fatalf("older list mismatch: %#v", runs)
	}
	if _, err := st.GetRun(oldID); err != nil {
		t.Fatalf("dry-run list deleted run: %v", err)
	}
}

func TestStats(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	day1 := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	successID := createStatsRun(t, st, StatusSuccess, "go test ./...", `["go","test","./..."]`, day1, 1000)
	failedID := createStatsRun(t, st, StatusFailed, "make lint", `["make","lint"]`, day1.Add(time.Hour), 2500)
	if successID == failedID {
		t.Fatalf("expected unique ids")
	}
	if _, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "npm run dev",
		ArgvJSON:  `["npm","run","dev"]`,
		CWD:       "/tmp/project",
		StartedAt: day2,
	}); err != nil {
		t.Fatalf("create running run: %v", err)
	}

	stats, err := st.Stats(2)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalRuns != 3 || stats.CompletedRuns != 2 || stats.SuccessfulRuns != 1 || stats.FailedRuns != 1 || stats.RunningRuns != 1 {
		t.Fatalf("stats counts mismatch: %#v", stats)
	}
	if stats.ActiveDays != 2 {
		t.Fatalf("active days = %d, want 2", stats.ActiveDays)
	}
	if stats.AvgDurationMS == nil || *stats.AvgDurationMS != 1750 {
		t.Fatalf("avg duration = %#v, want 1750", stats.AvgDurationMS)
	}
	if stats.FirstStartedAt == nil || !stats.FirstStartedAt.Equal(day1) {
		t.Fatalf("first started = %#v, want %s", stats.FirstStartedAt, day1)
	}
	if stats.LastStartedAt == nil || !stats.LastStartedAt.Equal(day2) {
		t.Fatalf("last started = %#v, want %s", stats.LastStartedAt, day2)
	}
	if len(stats.SlowestRuns) != 2 {
		t.Fatalf("slowest length = %d, want 2", len(stats.SlowestRuns))
	}
	if stats.SlowestRuns[0].ID != failedID || stats.SlowestRuns[0].DurationMS != 2500 {
		t.Fatalf("first slowest mismatch: %#v", stats.SlowestRuns[0])
	}
	if stats.SlowestRuns[1].ID != successID || stats.SlowestRuns[1].DurationMS != 1000 {
		t.Fatalf("second slowest mismatch: %#v", stats.SlowestRuns[1])
	}

	withoutSlowest, err := st.Stats(0)
	if err != nil {
		t.Fatalf("stats without slowest: %v", err)
	}
	if len(withoutSlowest.SlowestRuns) != 0 {
		t.Fatalf("slowest should be empty: %#v", withoutSlowest.SlowestRuns)
	}
}

// ---------------------------------------------------------------------------
// All / AllRuns
// ---------------------------------------------------------------------------

func TestAll(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	ids := make([]int64, 5)
	for i := 0; i < 5; i++ {
		id, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "cmd",
			ArgvJSON:  `["cmd"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		ids[i] = id
	}

	// limit=3 should return the 3 most recent
	runs, err := st.All(3)
	if err != nil {
		t.Fatalf("All(3): %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("All(3): want 3 runs, got %d", len(runs))
	}
	// newest-first
	if runs[0].ID != ids[4] {
		t.Fatalf("All(3): expected newest first, got id=%d", runs[0].ID)
	}

	// limit=100 should return all 5
	all, err := st.All(100)
	if err != nil {
		t.Fatalf("All(100): %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("All(100): want 5 runs, got %d", len(all))
	}

	// empty store
	st2, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open empty store: %v", err)
	}
	defer func() { _ = st2.Close() }()
	empty, err := st2.All(10)
	if err != nil {
		t.Fatalf("All on empty store: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("All on empty store: want 0 runs, got %d", len(empty))
	}
}

func TestAllRuns(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	const n = 4
	for i := 0; i < n; i++ {
		if _, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "allruns",
			ArgvJSON:  `["allruns"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
	}

	runs, err := st.AllRuns()
	if err != nil {
		t.Fatalf("AllRuns: %v", err)
	}
	if len(runs) != n {
		t.Fatalf("AllRuns: want %d runs, got %d", n, len(runs))
	}
	// newest-first ordering
	for i := 1; i < len(runs); i++ {
		if runs[i].ID >= runs[i-1].ID {
			t.Fatalf("AllRuns: not sorted newest-first at index %d", i)
		}
	}

	// empty store
	st2, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open empty store: %v", err)
	}
	defer func() { _ = st2.Close() }()
	empty, err := st2.AllRuns()
	if err != nil {
		t.Fatalf("AllRuns on empty store: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("AllRuns on empty store: want 0, got %d", len(empty))
	}
}

// ---------------------------------------------------------------------------
// ListHistory — filter fields
// ---------------------------------------------------------------------------

func TestListHistoryFilters(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	// run A: success, main branch, /home/alice, go test, started at base
	idA := createStatsRun(t, st, StatusSuccess, "go test ./...", `["go","test","./..."]`, base, 500)
	if err := st.SetLabel(idA, "ci"); err != nil {
		t.Fatalf("set label A: %v", err)
	}
	// override CWD and branch by reading then re-inserting isn't possible via helpers,
	// so we patch the DB directly for the filter fields that aren't covered by createStatsRun.
	if _, err := st.db.Exec(`UPDATE runs SET cwd=?, git_branch=? WHERE id=?`, "/home/alice/project", "main", idA); err != nil {
		t.Fatalf("patch run A: %v", err)
	}

	// run B: failed, feature branch, /home/bob, make lint, started 2 hours later
	idB := createStatsRun(t, st, StatusFailed, "make lint", `["make","lint"]`, base.Add(2*time.Hour), 200)
	if _, err := st.db.Exec(`UPDATE runs SET cwd=?, git_branch=? WHERE id=?`, "/home/bob/project", "feature/x", idB); err != nil {
		t.Fatalf("patch run B: %v", err)
	}

	// --- filter by Status ---
	runs, err := st.ListHistory(10, HistoryFilter{Status: StatusSuccess})
	if err != nil {
		t.Fatalf("filter Status: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idA {
		t.Fatalf("filter Status=success: got %v", ids(runs))
	}

	runs, err = st.ListHistory(10, HistoryFilter{Status: StatusFailed})
	if err != nil {
		t.Fatalf("filter Status failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idB {
		t.Fatalf("filter Status=failed: got %v", ids(runs))
	}

	// --- filter by Since ---
	since := base.Add(time.Hour)
	runs, err = st.ListHistory(10, HistoryFilter{Since: since})
	if err != nil {
		t.Fatalf("filter Since: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idB {
		t.Fatalf("filter Since: got %v", ids(runs))
	}

	// --- filter by CWD (substring) ---
	runs, err = st.ListHistory(10, HistoryFilter{CWD: "alice"})
	if err != nil {
		t.Fatalf("filter CWD: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idA {
		t.Fatalf("filter CWD=alice: got %v", ids(runs))
	}

	// --- filter by Branch (exact) ---
	runs, err = st.ListHistory(10, HistoryFilter{Branch: "main"})
	if err != nil {
		t.Fatalf("filter Branch: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idA {
		t.Fatalf("filter Branch=main: got %v", ids(runs))
	}

	// --- filter by Command (substring) ---
	runs, err = st.ListHistory(10, HistoryFilter{Command: "lint"})
	if err != nil {
		t.Fatalf("filter Command: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idB {
		t.Fatalf("filter Command=lint: got %v", ids(runs))
	}

	// --- filter by Label (substring) ---
	runs, err = st.ListHistory(10, HistoryFilter{Label: "ci"})
	if err != nil {
		t.Fatalf("filter Label: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idA {
		t.Fatalf("filter Label=ci: got %v", ids(runs))
	}

	// --- no filter returns all ---
	runs, err = st.ListHistory(10, HistoryFilter{})
	if err != nil {
		t.Fatalf("no filter: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("no filter: want 2, got %d", len(runs))
	}

	// --- limit respected ---
	runs, err = st.ListHistory(1, HistoryFilter{})
	if err != nil {
		t.Fatalf("limit 1: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("limit 1: got %d", len(runs))
	}

	// --- combined filters ---
	runs, err = st.ListHistory(10, HistoryFilter{Status: StatusSuccess, Branch: "main"})
	if err != nil {
		t.Fatalf("combined filter: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != idA {
		t.Fatalf("combined filter: got %v", ids(runs))
	}
}

// ids extracts IDs for error messages.
func ids(runs []Run) []int64 {
	out := make([]int64, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

// ---------------------------------------------------------------------------
// SetLabel
// ---------------------------------------------------------------------------

func TestSetLabel(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "echo hi",
		ArgvJSON:  `["echo","hi"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	// set a label
	if err := st.SetLabel(id, "my-label"); err != nil {
		t.Fatalf("SetLabel: %v", err)
	}
	run, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Label != "my-label" {
		t.Fatalf("Label after set: got %q, want %q", run.Label, "my-label")
	}

	// clear the label (empty string)
	if err := st.SetLabel(id, ""); err != nil {
		t.Fatalf("SetLabel clear: %v", err)
	}
	run, err = st.GetRun(id)
	if err != nil {
		t.Fatalf("GetRun after clear: %v", err)
	}
	if run.Label != "" {
		t.Fatalf("Label after clear: got %q, want empty", run.Label)
	}
}

// ---------------------------------------------------------------------------
// CountStaleRuns / MarkStaleRunsKilled
// ---------------------------------------------------------------------------

func TestCountAndMarkStaleRuns(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now().UTC()

	// fresh running run — should not be stale (started 5 seconds ago, threshold 1h)
	_, err = st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "fresh",
		ArgvJSON:  `["fresh"]`,
		CWD:       "/tmp",
		StartedAt: now.Add(-5 * time.Second),
	})
	if err != nil {
		t.Fatalf("create fresh run: %v", err)
	}

	// two stale running runs — started 2h ago
	staleID1, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "stale1",
		ArgvJSON:  `["stale1"]`,
		CWD:       "/tmp",
		StartedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create stale run 1: %v", err)
	}
	staleID2, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   "stale2",
		ArgvJSON:  `["stale2"]`,
		CWD:       "/tmp",
		StartedAt: now.Add(-3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create stale run 2: %v", err)
	}

	// a completed (non-running) old run — should not be counted as stale
	createStatsRun(t, st, StatusSuccess, "done", `["done"]`, now.Add(-4*time.Hour), 100)

	n, err := st.CountStaleRuns(time.Hour)
	if err != nil {
		t.Fatalf("CountStaleRuns: %v", err)
	}
	if n != 2 {
		t.Fatalf("CountStaleRuns: want 2, got %d", n)
	}

	killed, err := st.MarkStaleRunsKilled(time.Hour)
	if err != nil {
		t.Fatalf("MarkStaleRunsKilled: %v", err)
	}
	if killed != 2 {
		t.Fatalf("MarkStaleRunsKilled: want 2, got %d", killed)
	}

	// verify both stale runs are now killed
	for _, id := range []int64{staleID1, staleID2} {
		run, err := st.GetRun(id)
		if err != nil {
			t.Fatalf("GetRun %d: %v", id, err)
		}
		if run.Status != StatusKilled {
			t.Fatalf("run %d: status=%q, want %q", id, run.Status, StatusKilled)
		}
	}

	// no more stale runs
	n, err = st.CountStaleRuns(time.Hour)
	if err != nil {
		t.Fatalf("CountStaleRuns after kill: %v", err)
	}
	if n != 0 {
		t.Fatalf("CountStaleRuns after kill: want 0, got %d", n)
	}

	// second call returns 0 affected rows
	killed, err = st.MarkStaleRunsKilled(time.Hour)
	if err != nil {
		t.Fatalf("MarkStaleRunsKilled second: %v", err)
	}
	if killed != 0 {
		t.Fatalf("MarkStaleRunsKilled second: want 0, got %d", killed)
	}
}

// ---------------------------------------------------------------------------
// ListOutsideKeepLast / DeleteKeepLast
// ---------------------------------------------------------------------------

func TestListOutsideKeepLast(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	var createdIDs []int64
	for i := 0; i < 5; i++ {
		id, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "cmd",
			ArgvJSON:  `["cmd"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		createdIDs = append(createdIDs, id)
	}

	// keepLast=3 → outside = oldest 2 (ids[0], ids[1])
	outside, err := st.ListOutsideKeepLast(3)
	if err != nil {
		t.Fatalf("ListOutsideKeepLast: %v", err)
	}
	if len(outside) != 2 {
		t.Fatalf("ListOutsideKeepLast: want 2, got %d", len(outside))
	}
	outsideSet := make(map[int64]bool)
	for _, r := range outside {
		outsideSet[r.ID] = true
	}
	if !outsideSet[createdIDs[0]] || !outsideSet[createdIDs[1]] {
		t.Fatalf("ListOutsideKeepLast: wrong runs: %v", ids(outside))
	}

	// keepLast >= total → nothing outside
	outside, err = st.ListOutsideKeepLast(10)
	if err != nil {
		t.Fatalf("ListOutsideKeepLast(10): %v", err)
	}
	if len(outside) != 0 {
		t.Fatalf("ListOutsideKeepLast(10): want 0, got %d", len(outside))
	}
}

func TestDeleteKeepLast(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	base := time.Now().UTC()
	var createdIDs []int64
	for i := 0; i < 5; i++ {
		id, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "keepcmd",
			ArgvJSON:  `["keepcmd"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		createdIDs = append(createdIDs, id)
	}

	// keepLast=3 → delete oldest 2
	deleted, err := st.DeleteKeepLast(3)
	if err != nil {
		t.Fatalf("DeleteKeepLast: %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("DeleteKeepLast: want 2 deleted, got %d", len(deleted))
	}

	// oldest two should be gone
	for _, id := range createdIDs[:2] {
		if _, err := st.GetRun(id); err == nil {
			t.Fatalf("DeleteKeepLast: run %d still exists", id)
		}
	}
	// newest three should survive
	for _, id := range createdIDs[2:] {
		if _, err := st.GetRun(id); err != nil {
			t.Fatalf("DeleteKeepLast: run %d missing: %v", id, err)
		}
	}

	// search index should not contain deleted runs
	results, err := st.SearchRuns("keepcmd", 10)
	if err != nil {
		t.Fatalf("SearchRuns after DeleteKeepLast: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("SearchRuns after DeleteKeepLast: want 3, got %d", len(results))
	}

	// keepLast >= remaining → no-op
	deleted, err = st.DeleteKeepLast(10)
	if err != nil {
		t.Fatalf("DeleteKeepLast no-op: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("DeleteKeepLast no-op: want 0, got %d", len(deleted))
	}
}

// ---------------------------------------------------------------------------
// nullableString (white-box unit tests)
// ---------------------------------------------------------------------------

func TestNullableString(t *testing.T) {
	if v := nullableString(""); v != nil {
		t.Fatalf("nullableString(%q): want nil, got %v", "", v)
	}
	if v := nullableString("hello"); v != "hello" {
		t.Fatalf("nullableString(%q): want %q, got %v", "hello", "hello", v)
	}
}

// ---------------------------------------------------------------------------
// parseTime (white-box unit tests)
// ---------------------------------------------------------------------------

func TestParseTime(t *testing.T) {
	// empty string must return error
	_, err := parseTime("")
	if err == nil {
		t.Fatal("parseTime(empty): expected error, got nil")
	}

	// valid RFC3339Nano
	const ts = "2026-05-10T12:00:00.123456789Z"
	tm, err := parseTime(ts)
	if err != nil {
		t.Fatalf("parseTime(%q): %v", ts, err)
	}
	if tm.IsZero() {
		t.Fatalf("parseTime(%q): returned zero time", ts)
	}

	// invalid format
	_, err = parseTime("not-a-time")
	if err == nil {
		t.Fatal("parseTime(invalid): expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteOlderThan — edge cases for remaining branches
// ---------------------------------------------------------------------------

func TestDeleteOlderThanNoMatch(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// only recent run
	if _, err := st.CreateRun(CreateRun{
		Status:    StatusSuccess,
		Mode:      ModeRun,
		Command:   "recent",
		ArgvJSON:  `["recent"]`,
		CWD:       "/tmp",
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// cutoff in the past — nothing should be deleted
	deleted, err := st.DeleteOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("DeleteOlderThan: want 0 deleted, got %d", len(deleted))
	}
}

func TestDeleteOlderThanAllMatch(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// three old runs
	base := time.Now().Add(-72 * time.Hour).UTC()
	for i := 0; i < 3; i++ {
		if _, err := st.CreateRun(CreateRun{
			Status:    StatusSuccess,
			Mode:      ModeRun,
			Command:   "oldone",
			ArgvJSON:  `["oldone"]`,
			CWD:       "/tmp",
			StartedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
	}

	deleted, err := st.DeleteOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if len(deleted) != 3 {
		t.Fatalf("DeleteOlderThan: want 3 deleted, got %d", len(deleted))
	}

	// store should be empty
	all, err := st.AllRuns()
	if err != nil {
		t.Fatalf("AllRuns: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("AllRuns after full delete: want 0, got %d", len(all))
	}

	// search should also be empty
	results, err := st.SearchRuns("oldone", 10)
	if err != nil {
		t.Fatalf("SearchRuns: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchRuns after full delete: want 0, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------

func createStatsRun(t *testing.T, st *Store, status, command, argv string, started time.Time, durationMS int64) int64 {
	t.Helper()
	id, err := st.CreateRun(CreateRun{
		Status:    StatusRunning,
		Mode:      ModeRun,
		Command:   command,
		ArgvJSON:  argv,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create stats run: %v", err)
	}
	exitCode := 0
	if status != StatusSuccess {
		exitCode = 1
	}
	if err := st.FinishRun(id, status, exitCode, started.Add(time.Duration(durationMS)*time.Millisecond)); err != nil {
		t.Fatalf("finish stats run: %v", err)
	}
	return id
}
