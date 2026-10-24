package gitmeta

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Meta struct {
	Root   string
	Branch string
	Commit string
	Dirty  *bool
}

func Collect(cwd string) Meta {
	root, ok := git(cwd, "rev-parse", "--show-toplevel")
	if !ok {
		return Meta{}
	}
	// Git prints forward slashes on Windows too; match the run's directory.
	root = filepath.FromSlash(root)

	var (
		branch, commit string
		dirty          bool
		wg             sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		branch, _ = git(cwd, "branch", "--show-current")
	}()
	go func() {
		defer wg.Done()
		commit, _ = git(cwd, "rev-parse", "HEAD")
	}()
	go func() {
		defer wg.Done()
		status, _ := git(cwd, "status", "--porcelain")
		dirty = strings.TrimSpace(status) != ""
	}()
	wg.Wait()

	return Meta{Root: root, Branch: branch, Commit: commit, Dirty: &dirty}
}

func git(cwd string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// Dirty reports whether the work tree at cwd has uncommitted changes, or nil
// when git cannot tell.
func Dirty(cwd string) *bool {
	status, ok := git(cwd, "status", "--porcelain")
	if !ok {
		return nil
	}
	dirty := strings.TrimSpace(status) != ""
	return &dirty
}
