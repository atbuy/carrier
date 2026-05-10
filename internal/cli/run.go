package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
)

var exitProcess = os.Exit

// parseRunFlags scans leading carrier flags from args (before the child command)
// and updates app fields. Returns the remaining args (the child command + its args).
// Required because runCmd uses DisableFlagParsing to pass arbitrary flags through
// to the child process, which prevents cobra from parsing persistent flags that
// appear after the "run" subcommand name.
func (a *app) parseRunFlags(args []string) ([]string, error) {
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--notify":
			a.notify = true
			i++
		case arg == "-N" || arg == "--notify-always":
			a.notifyAlways = true
			i++
		case arg == "-q" || arg == "--quiet":
			a.quiet = true
			i++
		case arg == "--no-redact":
			a.noRedact = true
			i++
		case arg == "-t" || arg == "--timeout":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %q requires a value", arg)
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", args[i+1], err)
			}
			a.timeout = d
			i += 2
		case strings.HasPrefix(arg, "--timeout="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", arg, err)
			}
			a.timeout = d
			i++
		case strings.HasPrefix(arg, "-t="):
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "-t="))
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", arg, err)
			}
			a.timeout = d
			i++
		default:
			return args[i:], nil
		}
	}
	return args[i:], nil
}

func (a *app) runCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <command...>",
		Short:              "run and record one command",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			args, err = a.parseRunFlags(args)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return cobra.MinimumNArgs(1)(cmd, args)
			}
			code, err := runner.Run(a.cfg, a.st, runner.Options{
				Mode: store.ModeRun, Argv: args, Notify: a.notify, NotifyAlways: a.notifyAlways,
				NoRedact: a.noRedact, Quiet: a.quiet, Timeout: a.timeout,
			})
			if err != nil {
				return err
			}
			exitProcess(code)
			return nil
		},
	}
}

func (a *app) rerunCmd() *cobra.Command {
	var edit bool
	cmd := &cobra.Command{
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
			if edit {
				argv, err = editArgv(argv)
				if err != nil {
					return err
				}
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
		},
	}
	cmd.Flags().BoolVarP(&edit, "edit", "e", false, "open command in $EDITOR before rerunning")
	return cmd
}

// editArgv opens argv in $EDITOR as a JSON array. Returns the edited argv,
// or an error if the editor exits non-zero or the user leaves the content unchanged.
func editArgv(argv []string) ([]string, error) {
	editorEnv := os.Getenv("EDITOR")
	if editorEnv == "" {
		editorEnv = os.Getenv("VISUAL")
	}
	if editorEnv == "" {
		return nil, errors.New("$EDITOR is not set")
	}

	f, err := os.CreateTemp("", "carrier-rerun-*.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(argv); err != nil {
		_ = f.Close()
		return nil, err
	}
	original, _ := os.ReadFile(f.Name())
	_ = f.Close()

	editorArgs := strings.Fields(editorEnv)
	editorArgs = append(editorArgs, f.Name())
	editorCmd := exec.Command(editorArgs[0], editorArgs[1:]...)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}

	edited, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	if string(edited) == string(original) {
		return nil, errors.New("command unchanged, aborting rerun")
	}

	var newArgv []string
	if err := json.Unmarshal(edited, &newArgv); err != nil {
		return nil, fmt.Errorf("invalid JSON after edit: %w", err)
	}
	if len(newArgv) == 0 {
		return nil, errors.New("command is empty after edit")
	}
	return newArgv, nil
}
