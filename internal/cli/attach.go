package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "attach <id-or-label>",
		Aliases: []string{"a"},
		Short:   "attach to an existing shell session",
		Long: `Start a new PTY shell that records commands into an existing session.

  carrier attach 5          # attach by session ID
  carrier attach mylabel    # attach by session label`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := resolveSession(a.st, args[0])
			if err != nil {
				return err
			}
			_ = a.st.ReopenSession(sess.ID)
			runErr := carriershell.Run(a.cfg, a.notify, a.notifyAlways, a.noRedact, sess.ID, sess.Label)
			_ = a.st.EndSession(sess.ID, time.Now())
			return runErr
		},
	}
}

// resolveSession finds a session by integer ID or label string.
func resolveSession(st *store.Store, arg string) (*store.Session, error) {
	if id, err := parseID(arg); err == nil {
		sess, err := st.GetSession(id)
		if err != nil {
			return nil, fmt.Errorf("session %d not found: %w", id, err)
		}
		return sess, nil
	}
	sess, err := st.FindSessionByLabel(arg)
	if err != nil {
		return nil, fmt.Errorf("no session with label %q: %w", arg, err)
	}
	return sess, nil
}
