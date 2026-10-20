package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func createFailedRunWithStderr(t *testing.T, st *store.Store, dir, command, stderr string, started time.Time) int64 {
	t.Helper()
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeRun, Command: command, ArgvJSON: `["sh"]`,
		CWD: "/tmp", StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.stderr", id))
	if err := os.WriteFile(path, []byte(stderr), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := st.UpdatePaths(id, "", path, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, store.StatusFailed, 1, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return id
}

func TestSimilarCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	past := createFailedRunWithStderr(t, st, dir, "make db", "dial tcp 127.0.0.1:5432: connect: connection refused\n", old)
	if _, err := st.CreateNote(store.CreateNote{RunID: &past, Text: "db down", CreatedAt: old}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	current := createFailedRunWithStderr(t, st, dir, "make api", "dial tcp 10.0.0.2:6543: connect: connection refused\n", time.Now())
	other := createFailedRunWithStderr(t, st, dir, "make docs", "permission denied writing site directory\n", time.Now())
	a := &app{st: st}

	out, _, err := runEmpty(t, a.similarCmd(), []string{fmt.Sprint(current)}, nil)
	if err != nil {
		t.Fatalf("similar: %v", err)
	}
	if !strings.HasPrefix(out, fmt.Sprintf("%d  failed", past)) || !strings.Contains(out, "2 days ago · 1 note") || strings.Contains(out, fmt.Sprintf("%d  failed", other)) {
		t.Fatalf("unexpected similar output:\n%s", out)
	}

	out, _, err = runEmpty(t, a.similarCmd(), []string{fmt.Sprint(current)}, map[string]string{"json": "true"})
	if err != nil {
		t.Fatalf("similar --json: %v", err)
	}
	var views []struct {
		ID        int64 `json:"id"`
		NoteCount int   `json:"note_count"`
	}
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}
	if len(views) != 1 || views[0].ID != past || views[0].NoteCount != 1 {
		t.Fatalf("unexpected JSON: %+v", views)
	}

	out, errOut, err := runEmpty(t, a.similarCmd(), []string{fmt.Sprint(other)}, nil)
	if err != nil || out != "" || !strings.Contains(errOut, fmt.Sprintf("no earlier failed runs share the error in run %d", other)) {
		t.Fatalf("no-match case: out=%q err=%q runErr=%v", out, errOut, err)
	}

	if _, _, err := runEmpty(t, a.similarCmd(), []string{"999"}, nil); err == nil || err.Error() != "run 999 not found" {
		t.Fatalf("missing run: got %v", err)
	}
}

func TestOnShellRunEnd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	dir := t.TempDir()
	createFailedRunWithStderr(t, st, dir, "old", "fatal: unable to access remote repository\n", time.Now().Add(-48*time.Hour))

	// A shell run whose log was still buffered when the hooks indexed it.
	logPath := filepath.Join(dir, "terminal.log")
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "git pull", ArgvJSON: `["git pull"]`,
		CWD: "/tmp", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	_ = st.UpdatePaths(id, "", "", logPath)
	if err := st.FinishRun(id, store.StatusFailed, 128, time.Now()); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("fatal: unable to access remote repository\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	cfg := config.Default()
	a := &app{st: st, cfg: cfg}
	if got := a.onShellRunEnd(id); got != "" {
		t.Fatalf("hints are opt-in, got %q", got)
	}
	if res, _ := st.SearchRuns("unable", 10, store.SearchFilter{}); len(res) != 2 {
		t.Fatalf("run end should re-index the flushed log, got %d results", len(res))
	}

	a.cfg.Hints.SimilarFailures = true
	if got := a.onShellRunEnd(id); !strings.Contains(got, "carrier: seen before in run 1 (2 days ago): carrier show 1") {
		t.Fatalf("shell hint = %q", got)
	}

	ok, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "true", ArgvJSON: `["true"]`,
		CWD: "/tmp", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	_ = st.FinishRun(ok, store.StatusSuccess, 0, time.Now())
	if got := a.onShellRunEnd(ok); got != "" {
		t.Fatalf("successful runs should not hint, got %q", got)
	}
}
