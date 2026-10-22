package gitmeta

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Head reads the repository root, branch, and commit for cwd from the files
// in .git, without starting git, so it is fast enough to run before every
// shell command. It leaves Dirty unset, since that needs "git status". ok is
// false when cwd is not in a repository or the repository is laid out in a
// way Head does not read (such as the reftable format); use Collect then.
func Head(cwd string) (meta Meta, ok bool) {
	root, gitDir, found := findGitDir(cwd)
	if !found {
		return Meta{}, false
	}
	head, err := readLine(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return Meta{}, false
	}
	meta.Root = root
	ref, isRef := strings.CutPrefix(head, "ref: ")
	if !isRef {
		// Detached HEAD holds the commit itself.
		if !isCommitID(head) {
			return Meta{}, false
		}
		meta.Commit = head
		return meta, true
	}
	meta.Branch = strings.TrimPrefix(ref, "refs/heads/")
	commonDir := gitDir
	if common, err := readLine(filepath.Join(gitDir, "commondir")); err == nil {
		// Linked worktrees keep refs in the main repository.
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		commonDir = filepath.Clean(common)
	}
	commit, ok := resolveRef(gitDir, commonDir, ref)
	if !ok {
		// A new repository has a branch but no commit yet.
		if _, err := os.Stat(filepath.Join(commonDir, "reftable")); err == nil {
			return Meta{}, false
		}
		return meta, true
	}
	meta.Commit = commit
	return meta, true
}

// findGitDir walks up from cwd to the directory holding .git, and returns it
// and the git directory, following a .git file ("gitdir: ...") as linked
// worktrees and submodules use.
func findGitDir(cwd string) (root, gitDir string, ok bool) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", false
	}
	for {
		candidate := filepath.Join(dir, ".git")
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				return dir, candidate, true
			}
			line, err := readLine(candidate)
			if target, isLink := strings.CutPrefix(line, "gitdir: "); err == nil && isLink {
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				return dir, filepath.Clean(target), true
			}
			return "", "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// resolveRef finds the commit a ref points to: a loose ref file in the git
// directory or the shared one, else packed-refs.
func resolveRef(gitDir, commonDir, ref string) (string, bool) {
	for _, dir := range []string{gitDir, commonDir} {
		if line, err := readLine(filepath.Join(dir, filepath.FromSlash(ref))); err == nil && isCommitID(line) {
			return line, true
		}
	}
	f, err := os.Open(filepath.Join(commonDir, "packed-refs"))
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		id, name, found := strings.Cut(scanner.Text(), " ")
		if found && name == ref && isCommitID(id) {
			return id, true
		}
	}
	return "", false
}

func readLine(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line), nil
}

// isCommitID reports whether s is a full SHA-1 or SHA-256 object ID.
func isCommitID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
