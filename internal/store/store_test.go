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
