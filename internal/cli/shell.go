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
		Short:   "start a recorded shell session",
		Long: `Start a shell session that records every command line with its output, timing,
exit code, and Git details. Supports zsh, bash, and fish.

To record every terminal automatically, add the line printed by
"carrier init <shell>" to your shell config; it runs "carrier shell --auto".
An automatic session started in a directory pinned with "carrier session pin"
joins the pinned session instead.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.checkAutoSessions(cmd); err != nil {
				return err
			}
			// Check before a session is created that the shell could not
			// record into.
			if err := carriershell.Supported(); err != nil {
				return autoStartError(auto, err)
			}
			if len(args) == 1 && label == "" {
				label = args[0]
			}
			label = strings.TrimSpace(label)
			if program != "" {
				a.cfg.Shell.Program = program
			}
			a.expireAutoSessionsIfDue(cmd)
			a.cleanTempIfDue()
			var sessionID int64
			quiet := auto
			if pinned := a.autoPinnedSession(cmd, auto, label); pinned != nil {
				// Joining a pinned session is worth the banner: it says which
				// task this terminal records into.
				sessionID, label, quiet = pinned.ID, pinned.Label, false
				_ = a.st.ReopenSession(sessionID)
			} else {
				var err error
				sessionID, err = a.st.CreateSession(store.CreateSession{
					Label:     label,
					StartedAt: time.Now(),
					Auto:      auto,
				})
				if err != nil {
					return autoStartError(auto, err)
				}
			}
			last, runErr := carriershell.Run(a.cfg, a.shellOptions(sessionID, label, quiet))
			final := finalSession(sessionID, last)
			if !auto || !a.dropExitRun(final, last) {
				a.finishInFlightShellRun(last, runErr)
			}
			_ = a.st.EndSession(final, time.Now())
			if errors.Is(runErr, carriershell.ErrHungUp) {
				// Closing the terminal is how many shells end; there is no
				// one left to tell.
				if auto {
					a.deleteIfEmptyAuto(final)
				}
				return nil
			}
			if !auto {
				return runErr
			}
			// Opening and closing a terminal should not leave a session behind.
			a.deleteIfEmptyAuto(final)
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

// errAutoSessionsOff ends "carrier shell --auto" when shell.auto_sessions is
// off. carrier exits 1 without a message, so the line from "carrier init"
// carries on as a plain shell, as it does when carrier cannot start.
var errAutoSessionsOff = errors.New("automatic sessions are off (shell.auto_sessions)")

// checkAutoSessions returns errAutoSessionsOff for "carrier shell --auto"
// when automatic sessions are off.
func (a *app) checkAutoSessions(cmd *cobra.Command) error {
	// "shell" right under the root, or on its own in tests.
	if cmd.Name() != "shell" || (cmd.Parent() != nil && cmd.Parent().HasParent()) {
		return nil
	}
	if auto, _ := cmd.Flags().GetBool("auto"); auto && !a.cfg.Shell.AutoSessions {
		return errAutoSessionsOff
	}
	return nil
}

// autoPinnedSession returns the session pinned for the current directory when
// an automatic session starts without a label, or nil.
func (a *app) autoPinnedSession(cmd *cobra.Command, auto bool, label string) *store.Session {
	if !auto || label != "" {
		return nil
	}
	return a.pinnedSession(cmd)
}

func (a *app) shellOptions(sessionID int64, label string, quiet bool) carriershell.Options {
	return carriershell.Options{
		Notify: a.notify, NotifyAlways: a.notifyAlways, NoRedact: a.noRedact,
		SessionID: sessionID, Label: label, Quiet: quiet, OnRunEnd: a.onShellRunEnd,
	}
}

// dropExitRun deletes the run of the command that closed an automatic
// session ("exit", "logout"). Every terminal ends that way, so the run carries
// no information, and without it a session that ran nothing else is empty and
// is deleted. zsh and bash exit before their prompt hook, leaving the run in
// flight; fish finishes it, so then it is the session's latest run. It
// reports whether the in-flight run was deleted.
func (a *app) dropExitRun(sessionID int64, last carriershell.State) bool {
	var r *store.Run
	var err error
	if last.CurrentID != 0 {
		r, err = a.st.GetRun(last.CurrentID)
	} else {
		r, err = a.st.LatestSessionRun(sessionID)
	}
	if err != nil || !isExitCommand(r.Command) {
		return false
	}
	if _, err := a.st.DeleteByID(r.ID); err != nil {
		return false
	}
	removeIfSet(r.TerminalOutputPath)
	return r.ID == last.CurrentID
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
	// The terminal closed while the command ran: it was cut short by the
	// hangup, which exits with 128 + SIGHUP.
	if errors.Is(runErr, carriershell.ErrHungUp) {
		status, exitCode = store.StatusKilled, 129
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
