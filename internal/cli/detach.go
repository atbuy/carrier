package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

// detachHooksVersion is the first CARRIER_HOOKS_VERSION whose hooks stop
// recording when the session file names session 0.
const detachHooksVersion = 2

func (a *app) detachCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "detach",
		Aliases: []string{"dt"},
		Short:   "stop recording this shell",
		Long: `Stop recording the shell this runs in, without closing it. Its session is
ended, or deleted if it is an automatic session with nothing in it, and the
prompt stops showing it. Commands from then on are not recorded.

"carrier attach" records into a session again, in the same shell; "exit"
closes the terminal as before.

  carrier detach            # stop recording here
  carrier attach 12         # record into session 12 again`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			statePath := recordedShellState()
			if statePath == "" {
				return errors.New("this terminal is not a recorded shell")
			}
			sf := &carriershell.StateFile{Path: statePath}
			state := sf.Read()
			out := cmd.OutOrStdout()
			if state.Detached {
				_, _ = fmt.Fprintln(out, `carrier: this shell is not recording; "carrier attach" records into a session again`)
				return nil
			}
			if v, _ := strconv.Atoi(os.Getenv("CARRIER_HOOKS_VERSION")); v < detachHooksVersion {
				return errors.New(`this shell was started by an older carrier, whose hooks cannot stop recording; open a new terminal, or "exit" this one`)
			}
			// The detach command itself is not part of the session it leaves.
			a.dropShellRun(isSessionCommand)
			state = sf.Read()
			// A line such as "make; carrier detach" is still running; the hooks
			// finish its run as usual.
			if err := sf.Write(carriershell.State{CurrentID: state.CurrentID, CurrentLog: state.CurrentLog, Detached: true}); err != nil {
				return err
			}
			if err := carriershell.WriteSessionInfo(statePath, 0, ""); err != nil {
				return err
			}
			sess, _ := a.st.GetSession(state.SessionID)
			a.leaveSession(state.SessionID)
			switch {
			case sess == nil:
				_, _ = fmt.Fprintln(out, "carrier: stopped recording this shell")
			case a.sessionExists(sess.ID):
				_, _ = fmt.Fprintf(out, "carrier: stopped recording this shell; ended session %s\n", sessionName(sess))
				printHint(cmd, fmt.Sprintf("resume it with: carrier attach %d", sess.ID))
			default:
				_, _ = fmt.Fprintln(out, "carrier: stopped recording this shell; its session had nothing in it and was removed")
			}
			return nil
		},
	}
}

func (a *app) sessionExists(id int64) bool {
	_, err := a.st.GetSession(id)
	return err == nil
}
