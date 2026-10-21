package cli

import (
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func (a *app) attachCmd() *cobra.Command {
	var newShell bool
	cmd := &cobra.Command{
		Use:     "attach <id-or-label>",
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
  carrier attach 1234         # no session 1234, so labels are matched too`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := resolveSession(cmd, a.st, args[0])
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
