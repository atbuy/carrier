package gitmeta

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCollectOutsideGitRepoReturnsZeroMeta(t *testing.T) {
	meta := Collect(t.TempDir())

	if meta.Root != "" || meta.Branch != "" || meta.Commit != "" || meta.Dirty != nil {
		t.Fatalf("expected zero metadata outside git repo, got %#v", meta)
	}
}

func TestCollectInsideGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "initial commit")

	meta := Collect(dir)

	if meta.Root == "" {
		t.Fatal("expected non-empty Root")
	}
	if meta.Branch != "main" {
		t.Errorf("Branch = %q, want %q", meta.Branch, "main")
	}
	if meta.Commit == "" {
		t.Error("expected non-empty Commit")
	}
	if meta.Dirty == nil {
		t.Fatal("expected non-nil Dirty")
	}
	if *meta.Dirty {
		t.Error("fresh repo with no uncommitted changes should not be dirty")
	}

	// Write a file to make it dirty.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	run("add", ".")

	dirty := Collect(dir)
	if dirty.Dirty == nil || !*dirty.Dirty {
		t.Error("repo with staged changes should be dirty")
	}
}

func TestGitFunctionReturnsEmptyForError(t *testing.T) {
	out, ok := git(t.TempDir(), "rev-parse", "--show-toplevel")
	if ok {
		t.Fatalf("expected ok=false outside git repo, got out=%q", out)
	}
	if out != "" {
		t.Fatalf("expected empty string, got %q", out)
	}
}
