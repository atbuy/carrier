package store

import (
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
	if versionID != 2 || !isApplied {
		t.Fatalf("goose version mismatch: version=%d applied=%v", versionID, isApplied)
	}
	version, err := st.MigrationVersion()
	if err != nil {
		t.Fatalf("migration version: %v", err)
	}
	if version != 2 {
		t.Fatalf("migration version = %d", version)
	}

	if reopened, err := Open(dir); err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	} else {
		_ = reopened.Close()
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
