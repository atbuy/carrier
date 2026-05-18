package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
	"github.com/atbuy/carrier/internal/logs"
	"github.com/atbuy/carrier/internal/notify"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) internalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Hidden: true}
	cmd.AddCommand(a.internalBeginCmd(), a.internalEndCmd())
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
			if shouldIgnore(command, a.cfg.Shell.IgnoreCommands) {
				_ = sf.Write(carriershell.State{SessionID: state.SessionID})
				return nil
			}
			started := time.Now()
			host, _ := os.Hostname()
			git := gitmeta.Collect(cwd)
			argvJSON, _ := json.Marshal([]string{command})
			var sessionID *int64
			if state.SessionID != 0 {
				v := state.SessionID
				sessionID = &v
			}
			var envID *int64
			if envJSON := captureEnvCLI(a.cfg); envJSON != "" {
				if eid, err := a.st.InsertOrGetEnvironment(envJSON); err == nil {
					envID = &eid
				}
			}
			id, err := a.st.CreateRun(store.CreateRun{
				Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: string(argvJSON),
				CWD: cwd, StartedAt: started, Hostname: host, Shell: os.Getenv("SHELL"),
				GitRoot: git.Root, GitBranch: git.Branch, GitCommit: git.Commit, GitDirty: git.Dirty,
				NotifyRequested: os.Getenv("CARRIER_NOTIFY") == "1", NotifyAlways: os.Getenv("CARRIER_NOTIFY_ALWAYS") == "1",
				SessionID: sessionID, EnvID: envID,
			})
			if err != nil {
				return err
			}
			path := logs.TerminalPath(a.cfg.Storage.DataDir, id)
			if err := a.st.UpdatePaths(id, "", "", path); err != nil {
				return err
			}
			return sf.Write(carriershell.State{CurrentID: id, CurrentLog: path, SessionID: state.SessionID})
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
	allPatterns := append(logs.BuiltinPatterns(), cfg.Redaction.Patterns...)
	redactor := logs.NewRedactor(true, allPatterns)
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

func shouldIgnore(cmd string, ignore []string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return true
	}
	base := fields[0]
	if strings.HasPrefix(base, "_carrier_") {
		return true
	}
	if base == "carrier" && len(fields) > 1 && fields[1] == "internal" {
		return true
	}
	if strings.HasSuffix(base, "/carrier") && len(fields) > 1 && fields[1] == "internal" {
		return true
	}
	for _, item := range ignore {
		if base == item || strings.HasSuffix(base, "/"+item) {
			return true
		}
	}
	return false
}
