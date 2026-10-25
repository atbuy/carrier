package cli

import (
	"bytes"
	"os"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
	"github.com/atbuy/carrier/internal/tui"
)

func (a *app) browseCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "browse",
		Aliases: []string{"tui", "ui"},
		Short:   "browse runs and sessions interactively",
		Long: `Browse recorded runs and sessions in an interactive, full-screen viewer.

The Runs tab lists runs with a preview of their details and output; rerun,
label, or delete them without leaving the browser. The Sessions tab lists
sessions with their timeline, as "carrier session show" prints it: attach to
one, label it, keep it, or open its runs. Tab switches between them.

Rerun and attach run after the browser exits, in your normal terminal.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.AllRuns()
			if err != nil {
				return err
			}
			result, err := tui.Run(a.st, a.cfg, runs, a.browseSessions(cmd))
			if err != nil {
				return err
			}
			switch result.Action {
			case tui.ActionRerun:
				return a.rerunByID(result.RunID)
			case tui.ActionAttach:
				sess, err := a.st.GetSession(result.SessionID)
				if err != nil {
					return err
				}
				return a.attachTo(cmd, sess, false)
			}
			return nil
		},
	}
}

// browseSessions supplies the Sessions tab of "carrier browse".
func (a *app) browseSessions(cmd *cobra.Command) tui.Sessions {
	// The browser runs in the terminal, so the timeline is colored for it,
	// though it is drawn into a buffer first.
	t := newTheme(os.Stdout)
	return tui.Sessions{
		List: func() ([]store.SessionChoice, error) {
			return a.st.SessionChoices(store.SessionChoiceFilter{Limit: 2000})
		},
		Timeline: func(id int64, width int) string {
			sess, err := a.st.GetSession(id)
			if err != nil {
				return err.Error()
			}
			tl, err := a.loadTimeline(sess, timelineOptions{lines: 3})
			if err != nil {
				return err.Error()
			}
			var b bytes.Buffer
			printTimelineWith(&b, t, tl)
			return b.String()
		},
		Label: func(id int64, label string) error {
			if err := a.st.UpdateSessionLabel(id, label); err != nil {
				return err
			}
			if statePath := recordedShellState(); statePath != "" && id == currentSessionID() {
				_ = carriershell.WriteSessionInfo(statePath, id, label)
			}
			carriershell.RelabelShells(a.cfg.Storage.DataDir, id, label)
			return nil
		},
		Keep: a.st.SetSessionKept,
	}
}

// rerunByID reruns the original command of the given run in the current terminal.
func (a *app) rerunByID(id int64) error {
	r, err := a.getRun(id)
	if err != nil {
		return err
	}
	argv, err := parseArgv(r.ArgvJSON)
	if err != nil {
		return err
	}
	code, err := runner.Run(a.cfg, a.st, runner.Options{
		Mode: store.ModeRerun, Argv: argv, CWD: r.CWD, Notify: a.notify, NotifyAlways: a.notifyAlways,
		NoRedact: a.noRedact, Quiet: a.quiet, Timeout: a.timeout,
	})
	if err != nil {
		return err
	}
	exitProcess(code)
	return nil
}
