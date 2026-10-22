package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
	"github.com/atbuy/carrier/internal/logs"
	"github.com/atbuy/carrier/internal/notify"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) internalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Hidden: true}
	cmd.AddCommand(a.internalBeginCmd(), a.internalEndCmd(), a.internalOutcomeCmd())
	return cmd
}

func (a *app) internalBeginCmd() *cobra.Command {
	var statePath, command, cwd string
	cmd := &cobra.Command{
		Use:    "begin",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sf := &carriershell.StateFile{Path: statePath}
			state := sf.Read()
			if isHookCommand(command) {
				_ = sf.Write(carriershell.State{SessionID: state.SessionID})
				return nil
			}
			// Commands in shell.ignore_commands are recorded without output:
			// knowing that "vim config.yaml" ran matters, its screen does not.
			recordOutput := !outputIgnored(command, a.cfg.Shell.IgnoreCommands)
			started := time.Now()
			host, _ := os.Hostname()
			argvJSON, _ := json.Marshal([]string{command})
			var sessionID *int64
			if state.SessionID != 0 {
				v := state.SessionID
				sessionID = &v
				// Another terminal recording into this session may have
				// ended it when it closed; this one is still recording.
				_ = a.st.ReopenSession(v)
			}

			// Start git and env collection concurrently.
			gitCh := make(chan gitmeta.Meta, 1)
			envCh := make(chan *int64, 1)
			go func() { gitCh <- gitmeta.Collect(cwd) }()
			go func() {
				var envID *int64
				if envJSON := captureEnvCLI(a.cfg); envJSON != "" {
					if eid, err := a.st.InsertOrGetEnvironment(envJSON); err == nil {
						envID = &eid
					}
				}
				envCh <- envID
			}()

			// Create run immediately so we can write the state file without
			// waiting for git/env — the PTY loop starts logging sooner.
			id, err := a.st.CreateRun(store.CreateRun{
				Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: string(argvJSON),
				CWD: cwd, StartedAt: started, Hostname: host, Shell: os.Getenv("SHELL"),
				NotifyRequested: os.Getenv("CARRIER_NOTIFY") == "1", NotifyAlways: os.Getenv("CARRIER_NOTIFY_ALWAYS") == "1",
				SessionID: sessionID,
			})
			if err != nil {
				return err
			}
			path := ""
			if recordOutput {
				path = logs.TerminalPath(a.cfg.Storage.DataDir, id)
				if err := a.st.UpdatePaths(id, "", "", path); err != nil {
					return err
				}
			}
			if err := sf.Write(carriershell.State{CurrentID: id, CurrentLog: path, SessionID: state.SessionID}); err != nil {
				return err
			}

			// Drain goroutines and backfill metadata before this subprocess exits.
			git := <-gitCh
			envID := <-envCh
			_ = a.st.BackfillMeta(id, git.Root, git.Branch, git.Commit, git.Dirty, envID)
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", "", "state file")
	cmd.Flags().StringVar(&command, "cmd", "", "command")
	cmd.Flags().StringVar(&cwd, "cwd", "", "cwd")
	return cmd
}

func (a *app) internalEndCmd() *cobra.Command {
	var statePath string
	var exitCode int
	cmd := &cobra.Command{
		Use:    "end",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sf := &carriershell.StateFile{Path: statePath}
			state := sf.Read()
			if state.CurrentID == 0 {
				return nil
			}
			status := store.StatusSuccess
			if exitCode != 0 {
				status = store.StatusFailed
			}
			if err := a.st.FinishRun(state.CurrentID, status, exitCode, time.Now()); err != nil {
				return err
			}
			r, err := a.st.GetRun(state.CurrentID)
			if err == nil {
				if notifyErr := notify.MaybeSend(a.cfg, *r); notifyErr != nil {
					_, _ = fmt.Fprintf(os.Stderr, "carrier: notify: %s\n", notifyErr)
				}
			}
			return sf.Write(carriershell.State{SessionID: state.SessionID})
		},
	}
	cmd.Flags().StringVar(&statePath, "state", "", "state file")
	cmd.Flags().IntVar(&exitCode, "exit", 0, "exit code")
	return cmd
}

func captureEnvCLI(cfg config.Config) string {
	if !cfg.Storage.CaptureEnv {
		return ""
	}
	// Always redact env values with builtin + custom patterns regardless of
	// cfg.Redaction.Enabled — that flag governs stdout/stderr, not DB storage.
	redactor := logs.NewRedactorWithBuiltins(true, cfg.Redaction.Patterns)
	raw := os.Environ()
	m := make(map[string]string, len(raw))
	for _, kv := range raw {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = logs.RedactEnvValue(k, v, redactor)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// isHookCommand reports whether cmd is empty or one of carrier's own hook
// commands, which are never recorded.
func isHookCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return true
	}
	base := fields[0]
	if strings.HasPrefix(base, "_carrier_") {
		return true
	}
	return (base == "carrier" || strings.HasSuffix(base, "/carrier")) && len(fields) > 1 && fields[1] == "internal"
}

// outputIgnored reports whether cmd's program is in shell.ignore_commands, so
// its run is recorded without output.
func outputIgnored(cmd string, ignore []string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	base := fields[0]
	for _, item := range ignore {
		if base == item || strings.HasSuffix(base, "/"+item) {
			return true
		}
	}
	return false
}

// internalOutcomeCmd runs from the shell's exit hook when shell.ask_outcome is
// on. The shell still owns the terminal then, so the question can be asked
// and answered there.
func (a *app) internalOutcomeCmd() *cobra.Command {
	var statePath string
	cmd := &cobra.Command{
		Use:    "outcome",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return nil
			}
			state := (&carriershell.StateFile{Path: statePath}).Read()
			return a.askOutcome(state.SessionID, os.Stdin, os.Stderr)
		},
	}
	cmd.Flags().StringVar(&statePath, "state", "", "state file")
	return cmd
}

// askOutcome asks what a labeled session found, when it has no outcome yet,
// and records the answer. An empty answer skips it.
func (a *app) askOutcome(sessionID int64, in io.Reader, out io.Writer) error {
	if sessionID == 0 {
		return nil
	}
	sess, err := a.st.GetSession(sessionID)
	if err != nil || sess.Label == "" {
		return nil
	}
	if outcome, err := a.st.SessionOutcome(sessionID); err != nil || outcome != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "carrier: what did %q find? (Enter to skip) ", sess.Label)
	line, _ := bufio.NewReader(in).ReadString('\n')
	text := strings.TrimSpace(line)
	if text == "" {
		return nil
	}
	if _, err := a.st.CreateNote(store.CreateNote{SessionID: &sessionID, Text: text, Kind: store.NoteOutcome, CreatedAt: time.Now()}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "carrier: outcome saved; see it with \"carrier session show %d\"\n", sessionID)
	return nil
}
