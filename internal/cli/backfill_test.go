package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "one"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "changed.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func beginRun(t *testing.T, a *app, statePath, cwd string) *store.Run {
	t.Helper()
	cmd := a.internalBeginCmd()
	for k, v := range map[string]string{"state": statePath, "cmd": "make test", "cwd": cwd} {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("begin: %v", err)
	}
	r, err := a.st.Latest()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBeginRecordsGitAndBackfill(t *testing.T) {
	a, dir := autoSessionApp(t)
	repo := gitRepo(t)
	r := beginRun(t, a, filepath.Join(dir, "state.json"), repo)
	if r.GitRoot == "" || r.GitBranch != "main" || len(r.GitCommit) != 40 {
		t.Fatalf("branch and commit from .git: %+v", r)
	}
	if r.GitDirty == nil || !*r.GitDirty {
		t.Fatalf("dirty should be filled in by the backfill: %v", r.GitDirty)
	}
	if r.EnvJSON == "" {
		t.Fatal("the environment should be captured")
	}
}

func TestBackfillKeepsStartValues(t *testing.T) {
	a, dir := autoSessionApp(t)
	id, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "git checkout other", ArgvJSON: `["x"]`,
		CWD: dir, GitRoot: "/repo", GitBranch: "main", GitCommit: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	dirty := true
	if err := a.st.BackfillGit(id, "/repo", "other", "def", &dirty, nil); err != nil {
		t.Fatal(err)
	}
	r, _ := a.st.GetRun(id)
	if r.GitBranch != "main" || r.GitCommit != "abc" || r.GitDirty == nil || !*r.GitDirty {
		t.Fatalf("start values must be kept and dirty added: %+v", r)
	}
}

func TestEnvironmentCache(t *testing.T) {
	a, dir := autoSessionApp(t)
	statePath := filepath.Join(dir, "carrier-shell-1.json")
	first := a.cachedEnvironment(statePath)
	if first == nil {
		t.Fatal("no environment captured")
	}
	if b, err := os.ReadFile(statePath + ".env"); err != nil || len(b) < 64 {
		t.Fatalf("cache file = %q, %v", b, err)
	}
	if again := a.cachedEnvironment(statePath); again == nil || *again != *first {
		t.Fatalf("an unchanged environment should reuse %d, got %v", *first, again)
	}

	t.Setenv("CARRIER_TEST_CACHE", "changed")
	changed := a.cachedEnvironment(statePath)
	if changed == nil || *changed == *first {
		t.Fatalf("a changed environment needs a new row, got %v", changed)
	}

	// clean removes environments no run uses; the cache must not point there.
	if _, err := a.st.PruneOrphanedEnvironments(); err != nil {
		t.Fatal(err)
	}
	if a.st.EnvironmentExists(*changed) {
		t.Fatal("test setup: the environment should be pruned")
	}
	if fresh := a.cachedEnvironment(statePath); fresh == nil || !a.st.EnvironmentExists(*fresh) {
		t.Fatalf("a pruned environment should be captured again, got %v", fresh)
	}

	off := &app{cfg: config.Default(), st: a.st}
	off.cfg.Storage.CaptureEnv = false
	if off.cachedEnvironment(statePath) != nil {
		t.Fatal("capture_env = false captures nothing")
	}
}
