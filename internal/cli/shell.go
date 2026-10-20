package cli

import (
	"errors"
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
	var label string
	cmd := &cobra.Command{
		Use:     "shell",
		Aliases: []string{"s"},
		Short:   "start tracked shell (alpha)",
		Long:    "Start a PTY-backed tracked shell session. This command is alpha-quality and currently best-effort for zsh and bash.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && label == "" {
				label = args[0]
			}
			sessionID, err := a.st.CreateSession(store.CreateSession{
				Label:     strings.TrimSpace(label),
				StartedAt: time.Now(),
			})
			if err != nil {
				return err
			}
			last, runErr := carriershell.Run(a.cfg, a.notify, a.notifyAlways, a.noRedact, sessionID, strings.TrimSpace(label), a.onShellRunEnd)
			a.finishInFlightShellRun(last, runErr)
			_ = a.st.EndSession(sessionID, time.Now())
			return runErr
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "label for this shell session")
	return cmd
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
