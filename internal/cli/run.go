package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
)

// parseRunFlags scans leading carrier flags from args (before the child command)
// and updates app fields. Returns the remaining args (the child command + its args).
// Required because runCmd uses DisableFlagParsing to pass arbitrary flags through
// to the child process, which prevents cobra from parsing persistent flags that
// appear after the "run" subcommand name.
func (a *app) parseRunFlags(args []string) []string {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-n", "--notify":
			a.notify = true
			i++
		case "-N", "--notify-always":
			a.notifyAlways = true
			i++
		case "-q", "--quiet":
			a.quiet = true
			i++
		case "--no-redact":
			a.noRedact = true
			i++
		default:
			return args[i:]
		}
	}
	return args[i:]
}

func (a *app) runCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <command...>",
		Short:              "run and record one command",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			args = a.parseRunFlags(args)
			if len(args) == 0 {
				return cobra.MinimumNArgs(1)(cmd, args)
			}
			code, err := runner.Run(a.cfg, a.st, runner.Options{
				Mode: store.ModeRun, Argv: args, Notify: a.notify, NotifyAlways: a.notifyAlways,
				NoRedact: a.noRedact, Quiet: a.quiet,
			})
			if err != nil {
				return err
			}
			os.Exit(code)
			return nil
		},
	}
}

func (a *app) rerunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rerun <id>",
		Short: "rerun original command from original cwd",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
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
				NoRedact: a.noRedact, Quiet: a.quiet,
			})
			if err != nil {
				return err
			}
			os.Exit(code)
			return nil
		},
	}
}
