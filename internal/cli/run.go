package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/duration"
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
		case arg == "-L" || arg == "--label":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %q requires a value", arg)
			}
			a.label = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "--label="):
			a.label = strings.TrimPrefix(arg, "--label=")
			i++
		case strings.HasPrefix(arg, "-L="):
			a.label = strings.TrimPrefix(arg, "-L=")
			i++
		case arg == "-t" || arg == "--timeout":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %q requires a value", arg)
			}
			d, err := duration.Parse(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", args[i+1], err)
			}
			a.timeout = d
			i += 2
		case strings.HasPrefix(arg, "--timeout="):
			d, err := duration.Parse(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", arg, err)
			}
			a.timeout = d
			i++
		case strings.HasPrefix(arg, "-t="):
			d, err := duration.Parse(strings.TrimPrefix(arg, "-t="))
			if err != nil {
				return nil, fmt.Errorf("invalid timeout %q: %w", arg, err)
			}
			a.timeout = d
			i++
		case arg == "--retry" || arg == "--retry-delay":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %q requires a value", arg)
			}
			if err := a.setRetryFlag(arg, args[i+1]); err != nil {
				return nil, err
			}
			i += 2
		case strings.HasPrefix(arg, "--retry=") || strings.HasPrefix(arg, "--retry-delay="):
			name, value, _ := strings.Cut(arg, "=")
			if err := a.setRetryFlag(name, value); err != nil {
				return nil, err
			}
			i++
		default:
			return args[i:], nil
		}
	}
	return args[i:], nil
}

// setRetryFlag sets --retry or --retry-delay from value.
func (a *app) setRetryFlag(name, value string) error {
	if name == "--retry" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid --retry %q: give how many times to run a failing command again, such as 3", value)
		}
		a.retries = n
		return nil
	}
	d, err := duration.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid --retry-delay %q: %w", value, err)
	}
	a.retryDelay = d
	return nil
}

// runWithRetries runs a command, and again while it fails, up to a.retries
// more times. Each attempt is its own run. Only the last attempt notifies on
// failure, and Ctrl+C stops the retries. It returns the exit code of the last
// attempt.
func (a *app) runWithRetries(cmd *cobra.Command, opts runner.Options) (int, error) {
	attempts := a.retries + 1
	for attempt := 1; ; attempt++ {
		opts.Retrying = attempt < attempts
		res, err := runner.Execute(a.cfg, a.st, opts)
		if err != nil {
			return res.Exit, err
		}
		say := func(format string, args ...any) {
			if !a.quiet {
				printHint(cmd, fmt.Sprintf(format, args...))
			}
		}
		switch {
		case res.Status == store.StatusSuccess:
			if attempt > 1 {
				say("run %d succeeded on attempt %d of %d", res.RunID, attempt, attempts)
			}
			return res.Exit, nil
		case res.Interrupted:
			return res.Exit, nil
		case attempt == attempts:
			if attempts > 1 {
				say("run %d %s; gave up after %d attempts", res.RunID, howItFailed(res, opts), attempts)
			}
			return res.Exit, nil
		}
		wait := ""
		if a.retryDelay > 0 {
			wait = " in " + a.retryDelay.String()
		}
		say("run %d %s; retrying%s (attempt %d of %d)", res.RunID, howItFailed(res, opts), wait, attempt+1, attempts)
		if !sleepUnlessStopped(a.retryDelay) {
			return exitInterrupted, nil
		}
	}
}

// exitInterrupted is the exit code of a command stopped by Ctrl+C.
const exitInterrupted = 130

// howItFailed says how a run that did not succeed ended: "failed with exit
// 1", or "timed out after 30s".
func howItFailed(res runner.Result, opts runner.Options) string {
	if res.Status == store.StatusKilled && opts.Timeout > 0 {
		return "timed out after " + opts.Timeout.String()
	}
	return fmt.Sprintf("failed with exit %d", res.Exit)
}

// sleepUnlessStopped waits d, and reports false when Ctrl+C or a termination
// came first.
func sleepUnlessStopped(d time.Duration) bool {
	if d <= 0 {
		return true
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *app) runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run <command...>",
		Aliases: []string{"r"},
		Short:   "run and record one command",
		Long: `Run and record one command. carrier's flags go before the command; the
rest is passed to it as is.

  carrier run go test ./...
  carrier run -L BUG-1234 make e2e
  carrier run --retry 3 --retry-delay 5s npm ci   # up to 4 attempts`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if arg == "-h" || arg == "--help" {
					return cmd.Help()
				}
				if !strings.HasPrefix(arg, "-") {
					break
				}
			}
			var err error
			args, err = a.parseRunFlags(args)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return cobra.MinimumNArgs(1)(cmd, args)
			}
			code, err := a.runWithRetries(cmd, runner.Options{
				Mode: store.ModeRun, Argv: args, Label: a.label, Notify: a.notify, NotifyAlways: a.notifyAlways,
				NoRedact: a.noRedact, Quiet: a.quiet, Timeout: a.timeout,
			})
			if err != nil {
				return err
			}
			exitProcess(code)
			return nil
		},
	}
	// The command parses its own flags, to pass the rest on to the command
	// run; these are declared for its help.
	cmd.Flags().StringP("label", "L", "", "label for this run")
	cmd.Flags().DurationP("timeout", "t", 0, "stop the command after this long, such as 10m")
	cmd.Flags().Int("retry", 0, "run the command again while it fails, up to this many times")
	cmd.Flags().Duration("retry-delay", 0, "wait this long between attempts, such as 5s")
	return cmd
}

func (a *app) rerunCmd() *cobra.Command {
	var edit bool
	var label, retryDelay string
	cmd := &cobra.Command{
		Use:     "rerun <id>",
		Aliases: []string{"rr"},
		Short:   "rerun original command from original cwd",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.resolveRun(cmd, args[0])
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
			if retryDelay != "" {
				if err := a.setRetryFlag("--retry-delay", retryDelay); err != nil {
					return err
				}
			}
			if a.retries < 0 {
				return fmt.Errorf("invalid --retry %d: give how many times to run a failing command again, such as 3", a.retries)
			}
			code, err := a.runWithRetries(cmd, runner.Options{
				Mode: store.ModeRerun, Argv: argv, CWD: r.CWD, Label: label, Notify: a.notify, NotifyAlways: a.notifyAlways,
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
	cmd.Flags().StringVarP(&label, "label", "L", "", "label for this run")
	cmd.Flags().IntVar(&a.retries, "retry", 0, "run the command again while it fails, up to this many times")
	cmd.Flags().StringVar(&retryDelay, "retry-delay", "", "wait this long between attempts, such as 5s")
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
