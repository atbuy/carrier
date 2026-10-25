package shell

import (
	"os"
	"strings"
)

// BranchPath is where a recorded shell keeps the Git repository and branch it
// last saw after a command, next to its state file, so carrier can tell when
// a checkout moved it to another branch.
func BranchPath(statePath string) string { return statePath + ".branch" }

// ReadBranch returns the repository root and branch last written for the
// shell with this state file, or empty strings.
func ReadBranch(statePath string) (root, branch string) {
	b, err := os.ReadFile(BranchPath(statePath))
	if err != nil {
		return "", ""
	}
	root, branch, _ = strings.Cut(strings.TrimRight(string(b), "\n"), "\n")
	return root, branch
}

// WriteBranch records the repository root and branch the shell is on.
func WriteBranch(statePath, root, branch string) error {
	return os.WriteFile(BranchPath(statePath), []byte(root+"\n"+branch+"\n"), 0o600)
}
