package cli

import (
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
	"github.com/atbuy/carrier/internal/tui"
)

func (a *app) tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Aliases: []string{"ui"},
		Short:   "interactive run browser",
		Long: `Browse recorded runs in an interactive, full-screen viewer.

Navigate the run list, preview captured output, and rerun, label, or delete
runs. Rerun launches after the browser exits so the command runs in your normal
terminal.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.AllRuns()
			if err != nil {
				return err
			}
			result, err := tui.Run(a.st, a.cfg, runs)
			if err != nil {
				return err
			}
			if result.Action == tui.ActionRerun {
				return a.rerunByID(result.RunID)
			}
			return nil
		},
	}
}

// rerunByID reruns the original command of the given run in the current terminal.
func (a *app) rerunByID(id int64) error {
	r, err := a.st.GetRun(id)
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
