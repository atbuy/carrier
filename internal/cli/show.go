package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/logs"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) lastCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "last",
		Short: "show latest run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.st.Latest()
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd, runViewFromStore(r, false))
			}
			printRun(cmd.OutOrStdout(), r)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func (a *app) showCmd() *cobra.Command {
	var jsonOutput bool
	var lines int
	var onlyStdout, onlyStderr, showEnv bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "show full run details",
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
			if jsonOutput {
				redactor := logs.NewRedactor(a.cfg.Redaction.Enabled && !a.noRedact, a.cfg.Redaction.Patterns)
				return writeJSON(cmd, runViewFromStoreOpts(r, true, redactor))
			}
			out := cmd.OutOrStdout()
			c := outputColors(out)
			streamOnly := onlyStdout || onlyStderr
			if !streamOnly {
				printRun(out, r)
			}
			if r.TerminalOutputPath != "" && !onlyStderr {
				if !streamOnly {
					_, _ = fmt.Fprintln(out, "\n"+c.paint(colorBold+colorCyan, "terminal"))
				}
				_, _ = fmt.Fprint(out, lastNLines(readText(r.TerminalOutputPath), lines))
				return nil
			}
			if r.StdoutPath != "" && !onlyStderr {
				if !streamOnly {
					_, _ = fmt.Fprintln(out, "\n"+c.paint(colorBold+colorGreen, "stdout"))
				}
				_, _ = fmt.Fprint(out, lastNLines(readText(r.StdoutPath), lines))
			}
			if r.StderrPath != "" && !onlyStdout {
				if !streamOnly {
					_, _ = fmt.Fprintln(out, "\n"+c.paint(colorBold+colorRed, "stderr"))
				}
				_, _ = fmt.Fprint(out, lastNLines(readText(r.StderrPath), lines))
			}
			if showEnv && r.EnvJSON != "" {
				var env map[string]string
				if err := json.Unmarshal([]byte(r.EnvJSON), &env); err == nil {
					redactor := logs.NewRedactor(a.cfg.Redaction.Enabled && !a.noRedact, a.cfg.Redaction.Patterns)
					_, _ = fmt.Fprintln(out, "\n"+c.paint(colorBold+colorMagenta, "env"))
					for k, v := range env {
						_, _ = fmt.Fprintf(out, "  %s=%s\n", c.paint(colorCyan, k), string(redactor.Redact([]byte(v))))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVar(&lines, "lines", 0, "show only the last N lines of output (0 = all)")
	cmd.Flags().BoolVar(&onlyStdout, "stdout", false, "print only stdout (no header, no stderr)")
	cmd.Flags().BoolVar(&onlyStderr, "stderr", false, "print only stderr (no header, no stdout)")
	cmd.Flags().BoolVar(&showEnv, "env", false, "print captured environment variables")
	cmd.MarkFlagsMutuallyExclusive("stdout", "stderr")
	return cmd
}

// lastNLines returns the last n lines of s. Returns s unchanged when n <= 0.
func lastNLines(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	// Walk backwards counting newlines.
	end := len(s)
	// Ignore a trailing newline when counting.
	pos := end
	if pos > 0 && s[pos-1] == '\n' {
		pos--
	}
	found := 0
	for pos > 0 && found < n {
		pos--
		if s[pos] == '\n' {
			found++
		}
	}
	if found == n {
		pos++ // step past the newline we stopped on
	}
	return s[pos:end]
}

func (a *app) failedCmd() *cobra.Command {
	return listStatusCmd("failed", store.StatusFailed, a)
}

func (a *app) runningCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "running",
		Short: "list running runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.ListByStatus(store.StatusRunning, 100)
			if err != nil {
				return err
			}
			if jsonOutput {
				views := make([]runView, 0, len(runs))
				for _, r := range runs {
					views = append(views, runViewFromStore(&r, false))
				}
				return writeJSON(cmd, views)
			}
			out := cmd.OutOrStdout()
			c := outputColors(out)
			for _, r := range runs {
				ms := timeSinceMS(r.StartedAt)
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s  %s\n",
					c.paint(colorCyan, fmt.Sprintf("%d", r.ID)),
					c.paint(statusColor(r.Status), r.Status),
					c.paint(colorGreen, displayCommand(&r)),
					c.paint(colorGray, r.CWD),
					c.paint(colorCyan, formatDuration(&ms)),
				)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func listStatusCmd(name, status string, a *app) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "list " + name + " runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.ListByStatus(status, 100)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			c := outputColors(out)
			for _, r := range runs {
				age := ""
				if status == store.StatusRunning {
					ms := timeSinceMS(r.StartedAt)
					age = "  " + formatDuration(&ms)
				}
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s%s\n",
					c.paint(colorCyan, fmt.Sprintf("%d", r.ID)),
					c.paint(statusColor(r.Status), r.Status),
					c.paint(colorGreen, displayCommand(&r)),
					c.paint(colorGray, r.CWD),
					c.paint(colorCyan, age),
				)
			}
			return nil
		},
	}
}

func timeSinceMS(t time.Time) int64 {
	return time.Since(t).Milliseconds()
}

type runView struct {
	ID                 int64             `json:"id"`
	Status             string            `json:"status"`
	Mode               string            `json:"mode"`
	Command            string            `json:"command"`
	Argv               []string          `json:"argv,omitempty"`
	CWD                string            `json:"cwd"`
	StartedAt          time.Time         `json:"started_at"`
	FinishedAt         *time.Time        `json:"finished_at,omitempty"`
	DurationMS         *int64            `json:"duration_ms,omitempty"`
	ExitCode           *int              `json:"exit_code,omitempty"`
	Hostname           string            `json:"hostname,omitempty"`
	Shell              string            `json:"shell,omitempty"`
	GitRoot            string            `json:"git_root,omitempty"`
	GitBranch          string            `json:"git_branch,omitempty"`
	GitCommit          string            `json:"git_commit,omitempty"`
	GitDirty           *bool             `json:"git_dirty,omitempty"`
	StdoutPath         string            `json:"stdout_path,omitempty"`
	StderrPath         string            `json:"stderr_path,omitempty"`
	TerminalOutputPath string            `json:"terminal_output_path,omitempty"`
	Stdout             string            `json:"stdout,omitempty"`
	Stderr             string            `json:"stderr,omitempty"`
	TerminalOutput     string            `json:"terminal_output,omitempty"`
	NotifyRequested    bool              `json:"notify_requested"`
	NotifyAlways       bool              `json:"notify_always"`
	CreatedAt          time.Time         `json:"created_at"`
	Label              string            `json:"label,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
}

func runViewFromStore(r *store.Run, includeOutput bool) runView {
	return runViewFromStoreOpts(r, includeOutput, logs.Redactor{})
}

func runViewFromStoreOpts(r *store.Run, includeOutput bool, redactor logs.Redactor) runView {
	argv, _ := parseArgv(r.ArgvJSON)
	view := runView{
		ID: r.ID, Status: r.Status, Mode: r.Mode, Command: displayCommand(r), Argv: argv, CWD: r.CWD,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, DurationMS: r.DurationMS, ExitCode: r.ExitCode,
		Hostname: r.Hostname, Shell: r.Shell, GitRoot: r.GitRoot, GitBranch: r.GitBranch, GitCommit: r.GitCommit,
		GitDirty: r.GitDirty, StdoutPath: r.StdoutPath, StderrPath: r.StderrPath, TerminalOutputPath: r.TerminalOutputPath,
		NotifyRequested: r.NotifyRequested, NotifyAlways: r.NotifyAlways, CreatedAt: r.CreatedAt,
		Label: r.Label,
	}
	if includeOutput {
		view.Stdout = readText(r.StdoutPath)
		view.Stderr = readText(r.StderrPath)
		view.TerminalOutput = readText(r.TerminalOutputPath)
		if r.EnvJSON != "" {
			var raw map[string]string
			if err := json.Unmarshal([]byte(r.EnvJSON), &raw); err == nil {
				env := make(map[string]string, len(raw))
				for k, v := range raw {
					env[k] = string(redactor.Redact([]byte(v)))
				}
				view.Env = env
			}
		}
	}
	return view
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
