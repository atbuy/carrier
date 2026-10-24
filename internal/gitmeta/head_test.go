package gitmeta

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestHeadMatchesGit checks Head against git itself in the layouts it reads.
func TestHeadMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "sub", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q", "-b", "main")

	check := func(name, cwd string) {
		t.Helper()
		got, ok := Head(cwd)
		want := Collect(cwd)
		if !ok || !sameDir(got.Root, want.Root) || got.Branch != want.Branch || got.Commit != want.Commit || got.Dirty != nil {
			t.Errorf("%s: Head = %+v ok=%v, git says root=%q branch=%q commit=%q", name, got, ok, want.Root, want.Branch, want.Commit)
		}
	}

	check("new repository", repo)
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	check("loose ref", repo)
	check("subdirectory", filepath.Join(repo, "sub", "dir"))
	runGit(t, repo, "checkout", "-q", "-b", "feature/BUG-1234")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "two")
	check("branch with a slash", repo)
	runGit(t, repo, "pack-refs", "--all")
	check("packed refs", repo)
	runGit(t, repo, "checkout", "-q", "--detach")
	check("detached HEAD", repo)
	runGit(t, repo, "checkout", "-q", "main")
	runGit(t, repo, "worktree", "add", "-q", "-b", "wt", filepath.Join(dir, "wt"))
	check("linked worktree", filepath.Join(dir, "wt"))
}

func TestHeadOutsideRepository(t *testing.T) {
	if _, ok := Head(t.TempDir()); ok {
		t.Fatal("a directory outside any repository has no HEAD")
	}
}

func TestIsCommitID(t *testing.T) {
	for s, want := range map[string]bool{
		"0123456789abcdef0123456789abcdef01234567": true,
		"0123456789ABCDEF0123456789abcdef01234567": false,
		"main": false,
		"":     false,
	} {
		if got := isCommitID(s); got != want {
			t.Errorf("isCommitID(%q) = %v", s, got)
		}
	}
}

// sameDir reports whether two paths name the same directory. Head keeps the
// path it was given, while git resolves symlinks (macOS /var is /private/var)
// and, on Windows, short names such as RUNNER~1.
func sameDir(a, b string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	ra, rb := resolve(a), resolve(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ra, rb)
	}
	return ra == rb
}
