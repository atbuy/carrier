package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/hints"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) shellCmd() *cobra.Command {
	var label, program string
	var auto bool
	cmd := &cobra.Command{
		Use:     "shell",
		Aliases: []string{"s"},
		Short:   "start tracked shell (alpha)",
		Long: `Start a PTY-backed tracked shell session. This command is alpha-quality and
currently best-effort for zsh, bash, and fish.

To record every terminal automatically, add the line printed by
"carrier init <shell>" to your shell config; it runs "carrier shell --auto".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && label == "" {
				label = args[0]
			}
			label = strings.TrimSpace(label)
			if program != "" {
				a.cfg.Shell.Program = program
			}
			a.expireAutoSessionsIfDue(cmd)
			sessionID, err := a.st.CreateSession(store.CreateSession{
				Label:     label,
				StartedAt: time.Now(),
				Auto:      auto,
			})
			if err != nil {
				return autoStartError(auto, err)
			}
			last, runErr := carriershell.Run(a.cfg, a.shellOptions(sessionID, label, auto))
			if !auto || !a.dropExitRun(last) {
				a.finishInFlightShellRun(last, runErr)
			}
			_ = a.st.EndSession(sessionID, time.Now())
			if !auto {
				return runErr
			}
			// Opening and closing a terminal should not leave a session behind.
			if empty, err := a.st.SessionIsEmpty(sessionID); err == nil && empty {
				_, _ = a.st.DeleteSession(sessionID)
			}
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				// The user's shell ended; its status is not a carrier failure,
				// and exit 0 lets the shell config close the terminal.
				return nil
			}
			return autoStartError(auto, runErr)
		},
	}
	cmd.Flags().StringVarP(&label, "label", "L", "", "label for this shell session")
	cmd.Flags().BoolVar(&auto, "auto", false, "start an automatic session from a shell config (see carrier init)")
	cmd.Flags().StringVar(&program, "program", "", "shell program to run (default: shell.program, then $SHELL)")
	return cmd
}

func (a *app) shellOptions(sessionID int64, label string, quiet bool) carriershell.Options {
	return carriershell.Options{
		Notify: a.notify, NotifyAlways: a.notifyAlways, NoRedact: a.noRedact,
		SessionID: sessionID, Label: label, Quiet: quiet, OnRunEnd: a.onShellRunEnd,
	}
}

// dropExitRun deletes the in-flight run when it is the command that closed an
// automatic session ("exit", "logout"). Every terminal ends that way, so the
// run carries no information, and without it a session that ran nothing else
// is empty and is deleted.
func (a *app) dropExitRun(last carriershell.State) bool {
	if last.CurrentID == 0 {
		return false
	}
	r, err := a.st.GetRun(last.CurrentID)
	if err != nil || !isExitCommand(r.Command) {
		return false
	}
	if _, err := a.st.DeleteByID(r.ID); err != nil {
		return false
	}
	removeIfSet(r.TerminalOutputPath)
	return true
}

func isExitCommand(command string) bool {
	fields := strings.Fields(command)
	return len(fields) > 0 && len(fields) <= 2 && (fields[0] == "exit" || fields[0] == "logout")
}

// autoStartError explains a failed automatic session; the shell config then
// carries on as a plain shell.
func autoStartError(auto bool, err error) error {
	if !auto || err == nil {
		return err
	}
	return fmt.Errorf("automatic session not started: %w; continuing without carrier (CARRIER_NO_AUTO=1 skips it)", err)
}

// finishInFlightShellRun finishes the run that was still in flight when the
// shell exited. The shell exits before its prompt hook can end that run, so
// without this "exit" (or a command killed with the shell) stays running.
func (a *app) finishInFlightShellRun(last carriershell.State, runErr error) {
	if last.CurrentID == 0 {
		return
	}
	status, exitCode := store.StatusSuccess, 0
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
		switch {
		case exitCode < 0:
			status = store.StatusKilled
		case exitCode > 0:
			status = store.StatusFailed
		}
	}
	_ = a.st.FinishRun(last.CurrentID, status, exitCode, time.Now())
}

// onShellRunEnd runs in the carrier shell process once a command's terminal
// log is complete. The hooks index a run as soon as it ends, before the log is
// flushed, so it is re-indexed here for search; failed runs may get a hint.
func (a *app) onShellRunEnd(runID int64) string {
	_ = a.st.ReindexRun(runID)
	r, err := a.st.GetRun(runID)
	if err != nil || r.Status != store.StatusFailed || r.ExitCode == nil {
		return ""
	}
	hint := hints.SimilarFailure(a.st, a.cfg, runID, *r.ExitCode)
	if hint == "" {
		return ""
	}
	return newTheme(os.Stdout).Muted.Render("carrier: " + hint)
}
