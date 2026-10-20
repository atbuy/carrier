package cli

import (
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func (a *app) attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "attach <id-or-label>",
		Aliases: []string{"a"},
		Short:   "attach to an existing shell session",
		Long: `Start a new PTY shell that records commands into an existing session.

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
			_ = a.st.ReopenSession(sess.ID)
			last, runErr := carriershell.Run(a.cfg, a.notify, a.notifyAlways, a.noRedact, sess.ID, sess.Label, a.onShellRunEnd)
			a.finishInFlightShellRun(last, runErr)
			_ = a.st.EndSession(sess.ID, time.Now())
			return runErr
		},
	}
}
