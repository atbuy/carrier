package gitmeta

import (
	"os/exec"
	"strings"
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
	branch, _ := git(cwd, "branch", "--show-current")
	commit, _ := git(cwd, "rev-parse", "HEAD")
	status, _ := git(cwd, "status", "--porcelain")
	dirty := strings.TrimSpace(status) != ""
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
