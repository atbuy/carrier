package cli

import (
	"errors"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
	"github.com/atbuy/carrier/internal/tui"
)

func (a *app) attachCmd() *cobra.Command {
	var newShell, branch bool
	cmd := &cobra.Command{
		Use:     "attach [<id-or-label> | --branch]",
		Aliases: []string{"a"},
		Short:   "record into an existing shell session",
		Long: `Record commands into an existing session.

Inside a recorded shell, attach switches that shell to the session: commands
from the next one on are recorded there, and the prompt follows. The session
it leaves is ended, or deleted if it is an automatic session with nothing in
it. Outside a recorded shell, or with --new-shell, attach starts a new shell.

  carrier attach 5            # attach by session ID
  carrier attach mylabel      # attach by session label
  carrier attach BUG-1234    # words of a label: matches "BUG-1234: fix login"
  carrier attach 1234         # no session 1234, so labels are matched too
  carrier attach --branch     # the session of the Git branch checked out here
  carrier attach              # pick a session from a list

With --branch, attach uses the session linked to the branch checked out in the
current directory (see "carrier session branch"). Without one, it uses a
session labeled exactly like the branch, or starts a session named after it,
and links the branch to it.

Without a session, attach lists recent sessions to pick from: type to filter
them by ID or label, and press enter to attach. The session pinned here and
the one linked to the branch checked out here come first.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if branch {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.MaximumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var sess *store.Session
			var err error
			switch {
			case branch:
				sess, err = a.branchSession(cmd)
			case len(args) == 0:
				if sess, err = a.pickSession(cmd); sess == nil && err == nil {
					return nil
				}
			default:
				sess, err = resolveSession(cmd, a.st, args[0])
			}
			if err != nil {
				return err
			}
			if statePath := recordedShellState(); statePath != "" && !newShell {
				return a.switchSession(cmd, statePath, sess)
			}
			_ = a.st.ReopenSession(sess.ID)
			last, runErr := carriershell.Run(a.cfg, a.shellOptions(sess.ID, sess.Label, false))
			a.finishInFlightShellRun(last, runErr)
			_ = a.st.EndSession(finalSession(sess.ID, last), time.Now())
			return runErr
		},
	}
	cmd.Flags().BoolVar(&newShell, "new-shell", false, "start a new shell even inside a recorded shell")
	cmd.Flags().BoolVarP(&branch, "branch", "b", false, "attach to the session of the Git branch checked out here")
	return cmd
}

// finalSession is the session a recorded shell ended in: the one it started
// with, unless it switched sessions.
func finalSession(started int64, last carriershell.State) int64 {
	if last.SessionID != 0 {
		return last.SessionID
	}
	return started
}

// pickSession lets the user pick the session to attach to from recent ones.
// It returns nil when the picker is cancelled.
func (a *app) pickSession(cmd *cobra.Command) (*store.Session, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.New("give a session ID or label; in a terminal, carrier attach without one lists sessions to pick from")
	}
	f := store.SessionChoiceFilter{Limit: 500}
	if id := currentSessionID(); id != 0 {
		f.Exclude = []int64{id}
	}
	choices, err := a.st.SessionChoices(f)
	if err != nil {
		return nil, err
	}
	if len(choices) == 0 {
		if f.Exclude != nil {
			return nil, errors.New("no other sessions to attach to")
		}
		return nil, errors.New(`no sessions to attach to; "carrier shell" starts one`)
	}
	tags := map[int64][]string{}
	if pinned := a.pinnedSession(cmd); pinned != nil {
		tags[pinned.ID] = append(tags[pinned.ID], "pinned here")
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, branch, ok := currentBranch(cwd); ok {
			if linked, _ := a.st.BranchSession(root, branch); linked != nil {
				tags[linked.ID] = append(tags[linked.ID], "branch "+branch)
			}
		}
	}
	// Sessions tagged for where you are come first, then the newest.
	var tagged, rest []tui.SessionItem
	for _, c := range choices {
		item := tui.SessionItem{SessionChoice: c, Tags: tags[c.ID]}
		if len(item.Tags) > 0 {
			tagged = append(tagged, item)
		} else {
			rest = append(rest, item)
		}
	}
	id, ok, err := tui.PickSession(append(tagged, rest...), a.cfg.UI.Theme)
	if err != nil || !ok {
		return nil, err
	}
	return a.st.GetSession(id)
}
