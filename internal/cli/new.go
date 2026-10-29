package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) newCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "new [label...]",
		Aliases: []string{"n"},
		Short:   "start a new session for the next task",
		Long: `Start a new session, with a label when one is given, and record into it.

Inside a recorded shell, the shell switches to the new session in place: the
next commands are recorded there, and the prompt shows it. The session it
leaves is ended, or deleted if it is an automatic session with nothing in it;
"carrier attach" goes back to it. Outside a recorded shell, new starts a
recorded shell in the new session, as "carrier shell" does.

  carrier new BUG-5678: fix signup     # quotes are not needed
  carrier new                          # a session without a label
  carrier attach                       # go back to an earlier session`,
		RunE: func(cmd *cobra.Command, args []string) error {
			label := strings.Join(strings.Fields(strings.Join(args, " ")), " ")
			same := a.sessionLabeled(label)
			id, err := a.st.CreateSession(store.CreateSession{Label: label, StartedAt: time.Now()})
			if err != nil {
				return err
			}
			sess, err := a.st.GetSession(id)
			if err != nil {
				return err
			}
			if statePath := recordedShellState(); statePath != "" {
				return a.startHere(cmd, statePath, sess, same)
			}
			if same != nil {
				printHint(cmd, fmt.Sprintf("session %d is also labeled %q; carrier attach %d goes back to it", same.ID, label, same.ID))
			}
			return a.attachTo(cmd, sess, false)
		},
	}
}

// startHere moves this recorded shell into the new session sess.
func (a *app) startHere(cmd *cobra.Command, statePath string, sess, same *store.Session) error {
	// "carrier new" is bookkeeping, not part of either session.
	a.dropShellRun(isSessionCommand)
	sf := &carriershell.StateFile{Path: statePath}
	state := sf.Read()
	if err := a.moveShell(sf, state, sess); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "carrier: started session %s\n", sessionName(sess))
	switch {
	case same != nil:
		printHint(cmd, fmt.Sprintf("session %d is also labeled %q; carrier attach %d goes back to it", same.ID, sess.Label, same.ID))
	case state.SessionID != 0 && a.sessionExists(state.SessionID):
		printHint(cmd, fmt.Sprintf("go back to the last one with: carrier attach %d", state.SessionID))
	}
	if hooksIgnoreSessionInfo(statePath) {
		printOldHooksHint(cmd, state.SessionID)
	}
	return nil
}

// sessionLabeled returns a session already labeled label, ignoring case, or
// nil.
func (a *app) sessionLabeled(label string) *store.Session {
	if label == "" {
		return nil
	}
	labeled, err := a.st.ListLabeledSessions()
	if err != nil {
		return nil
	}
	for i := range labeled {
		if strings.EqualFold(strings.TrimSpace(labeled[i].Label), label) {
			return &labeled[i]
		}
	}
	return nil
}
