package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// openTerminal opens the terminal of the shell running carrier's hooks, whose
// output they discard, for the one line a branch change prints. Tests replace
// it.
var openTerminal = func() (io.WriteCloser, error) { return os.OpenFile(terminalDevice, os.O_WRONLY, 0) }

// canonicalRepoRoot is the form of a repository root that session-branch
// links are stored under: symlinks resolved, so every way of reaching the
// repository matches, and lower case on Windows, whose paths ignore case.
func canonicalRepoRoot(root string) string {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

// currentBranch returns the canonical root of the Git repository dir is in
// and its checked-out branch. ok is false outside a repository and on a
// detached HEAD, such as during a rebase.
func currentBranch(dir string) (root, branch string, ok bool) {
	head, inRepo := gitmeta.Head(dir)
	if !inRepo || head.Branch == "" {
		return "", "", false
	}
	return canonicalRepoRoot(head.Root), head.Branch, true
}

// noteRepoBranch runs before each command in a recorded shell. When the
// command starts in a repository the shell has not seen last, such as its
// first command or one after changing to another repository, it notes the
// branch there, so a checkout as that command is noticed after it. Within the
// same repository it leaves the note alone, so a branch switched between
// commands, say in an editor, is noticed too.
func (a *app) noteRepoBranch(statePath string, head gitmeta.Meta) {
	if a.cfg.Shell.BranchSessions == config.BranchOff || head.Root == "" || head.Branch == "" {
		return
	}
	root := canonicalRepoRoot(head.Root)
	if lastRoot, _ := carriershell.ReadBranch(statePath); lastRoot != root {
		_ = carriershell.WriteBranch(statePath, root, head.Branch)
	}
}

// noticeBranchChange runs after each command in a recorded shell. When a
// checkout moved the shell to another branch of the same repository, and that
// branch is linked to another session, it says so, or with
// shell.branch_sessions = "follow" switches the shell to that session. The
// first command in a new repository, leaving the repository, and a detached
// HEAD only update what the shell last saw; see noteRepoBranch.
func (a *app) noticeBranchChange(statePath string) {
	if a.cfg.Shell.BranchSessions == config.BranchOff {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	root, branch, ok := currentBranch(cwd)
	if !ok {
		return
	}
	lastRoot, lastBranch := carriershell.ReadBranch(statePath)
	if lastRoot == root && lastBranch == branch {
		return
	}
	_ = carriershell.WriteBranch(statePath, root, branch)
	if lastRoot != root || lastBranch == "" {
		return
	}
	sess, err := a.st.BranchSession(root, branch)
	sf := &carriershell.StateFile{Path: statePath}
	state := sf.Read()
	if err != nil || sess == nil || sess.ID == state.SessionID {
		return
	}
	term, err := openTerminal()
	if err != nil {
		term = nopWriteCloser{io.Discard}
	}
	defer func() { _ = term.Close() }()
	t := newTheme(term)
	if a.cfg.Shell.BranchSessions == config.BranchFollow {
		if err := a.moveShell(sf, state, sess); err != nil {
			return
		}
		_, _ = fmt.Fprintf(term, "%s branch %s → %s\n", t.Muted.Render("carrier:"), t.Label.Render(branch), sessionTitle(t, sess))
		return
	}
	_, _ = fmt.Fprintf(term, "%s branch %s has %s; switch with %s\n", t.Muted.Render("carrier:"),
		t.Label.Render(branch), sessionTitle(t, sess), t.Command.Render(fmt.Sprintf("carrier attach %d", sess.ID)))
}

// sessionTitle names a session with its ID and label, as "carrier status"
// does.
func sessionTitle(t theme, sess *store.Session) string {
	return t.Accent.Render(fmt.Sprintf("session %d", sess.ID)) + labelSuffix(t, sess.Label)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// errNotOnBranch is returned by the commands that need a checked-out branch.
var errNotOnBranch = errors.New("not on a Git branch here; check out a branch in a Git repository first")

// linkBranch links the branch checked out at root to sess. A linked automatic
// session is kept, as a pinned one is, so it does not expire.
func (a *app) linkBranch(cmd *cobra.Command, root, branch string, sess *store.Session) error {
	if err := a.st.LinkBranch(root, branch, sess.ID, time.Now()); err != nil {
		return err
	}
	if sess.Auto && !sess.Kept {
		if err := a.st.SetSessionKept(sess.ID, true); err == nil {
			printHint(cmd, "sessions linked to a branch are kept and never expire")
		}
	}
	return nil
}

// branchSession returns the session for the branch checked out in the
// current directory, for "carrier attach --branch": the linked one, else a
// session labeled exactly like the branch, else a new session named after it.
// The last two are linked to the branch.
func (a *app) branchSession(cmd *cobra.Command) (*store.Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, branch, ok := currentBranch(cwd)
	if !ok {
		return nil, errNotOnBranch
	}
	if sess, err := a.st.BranchSession(root, branch); err != nil || sess != nil {
		return sess, err
	}
	labeled, err := a.st.ListLabeledSessions()
	if err != nil {
		return nil, err
	}
	var sess *store.Session
	for i := range labeled {
		if strings.EqualFold(strings.TrimSpace(labeled[i].Label), branch) {
			sess = &labeled[i]
			break
		}
	}
	if sess == nil {
		id, err := a.st.CreateSession(store.CreateSession{Label: branch, StartedAt: time.Now()})
		if err != nil {
			return nil, err
		}
		if sess, err = a.st.GetSession(id); err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: started session %s for branch %s\n", sessionName(sess), branch)
	}
	if err := a.linkBranch(cmd, root, branch, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (a *app) sessionBranchCmd() *cobra.Command {
	var unlink bool
	cmd := &cobra.Command{
		Use:   "branch [<id-or-label>]",
		Short: "link the Git branch checked out here to a session",
		Long: `Link the Git branch checked out in the current directory to a session, by
default the session of this shell. A recorded shell that checks the branch out
later says which session it belongs to, or switches to it with
shell.branch_sessions = "follow". "carrier attach --branch" attaches to it.

  carrier session branch              # link this branch to this shell's session
  carrier session branch BUG-1234     # link it to a session by ID or label
  carrier session branch --unlink     # remove the link of this branch

A branch is linked to one session at a time; linking it again moves it. The
same branch name in another repository is a different branch.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, branch, ok := currentBranch(cwd)
			if !ok {
				return errNotOnBranch
			}
			out := cmd.OutOrStdout()
			if unlink {
				if len(args) > 0 {
					return errors.New("--unlink takes no session; it removes the link of the branch checked out here")
				}
				removed, err := a.st.UnlinkBranch(root, branch)
				if err != nil {
					return err
				}
				if !removed {
					_, _ = fmt.Fprintf(out, "carrier: branch %s is not linked to a session\n", branch)
					return nil
				}
				_, _ = fmt.Fprintf(out, "carrier: unlinked branch %s\n", branch)
				return nil
			}
			var sess *store.Session
			switch {
			case len(args) == 1:
				sess, err = resolveSession(cmd, a.st, args[0])
			case currentSessionID() != 0:
				sess, err = a.st.GetSession(currentSessionID())
			default:
				return errors.New(`give a session ID or label, or run this inside a recorded shell`)
			}
			if err != nil {
				return err
			}
			if err := a.linkBranch(cmd, root, branch, sess); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "carrier: linked branch %s to session %s\n", branch, sessionName(sess))
			return nil
		},
	}
	cmd.Flags().BoolVar(&unlink, "unlink", false, "remove the link of the branch checked out here")
	return cmd
}
