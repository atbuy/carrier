package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/notify"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/version"
)

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Aliases: []string{"d"},
		Short:   "check local carrier setup",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDoctor(cmd.OutOrStdout())
		},
	}
}

func (a *app) runDoctor(w io.Writer) error {
	t := newTheme(w)
	check(w, t, "version", true, version.Current().Version, nil)
	configPath, err := config.Path()
	check(w, t, "config path", err == nil, configPath, err)
	check(w, t, "data dir", dirWritable(a.cfg.Storage.DataDir), a.cfg.Storage.DataDir, nil)
	check(w, t, "data size", true, formatBytes(dirSize(a.cfg.Storage.DataDir)), nil)
	check(w, t, "runs dir", dirWritable(filepath.Join(a.cfg.Storage.DataDir, "runs")), filepath.Join(a.cfg.Storage.DataDir, "runs"), nil)
	check(w, t, "sqlite db", fileParentWritable(filepath.Join(a.cfg.Storage.DataDir, "carrier.db")), filepath.Join(a.cfg.Storage.DataDir, "carrier.db"), nil)
	if version, err := a.st.MigrationVersion(); err == nil {
		check(w, t, "migration", true, strconv.FormatInt(version, 10), nil)
	} else {
		check(w, t, "migration", false, "", err)
	}
	check(w, t, "max output", true, fmt.Sprintf("%d MB", a.cfg.Storage.MaxOutputMB), nil)
	check(w, t, "redaction", a.cfg.Redaction.Enabled, fmt.Sprintf("%d patterns", len(a.cfg.Redaction.Patterns)), nil)
	staleCount, _ := a.st.CountStaleRuns(a.cfg.StaleRunThreshold())
	check(w, t, "stale runs", staleCount == 0, fmt.Sprintf("%d stuck in running (threshold %s)", staleCount, a.cfg.Storage.StaleRunThreshold), nil)
	check(w, t, "git", commandAvailable("git"), "git executable available", nil)
	check(w, t, "notify", notify.Available(), "optional desktop notifications", nil)
	shellErr := carriershell.Supported()
	check(w, t, "shell", shellErr == nil && shellSupported(a.cfg.Shell.Program), shellProgram(a.cfg.Shell.Program), shellErr)
	check(w, t, "auto session", true, autoSessionStatus(a.cfg.Shell.Program, a.cfg.Shell.AutoSessionRetention), nil)
	s := a.terminalStatus()
	check(w, t, "this shell", s.shellHealthy(), thisShellDetail(s), nil)
	if s.Pin.Path != "" {
		detail := collapseHome(s.Pin.Path) + ": "
		if s.Pin.Err != nil {
			detail += s.Pin.Err.Error()
		} else {
			detail += "session " + sessionName(s.Pin.Session)
		}
		check(w, t, "pin", s.Pin.Err == nil, detail, nil)
	}
	if _, detail, applies := tmuxEnvironment(); applies {
		check(w, t, "tmux", s.TmuxStale == "", detail, nil)
	}
	leftover := carriershell.LeftoverTemp(time.Now())
	check(w, t, "temp files", true, fmt.Sprintf("%d left by shells that were killed; removed daily when a shell starts", leftover), nil)
	check(w, t, "terminal", term.IsTerminal(int(os.Stdout.Fd())), "stdout is a TTY", nil)
	return nil
}

func thisShellDetail(s terminalStatus) string {
	switch {
	case s.Inherited:
		return fmt.Sprintf("not recorded: it inherited session %d's variables from another terminal (tmux?); open a new terminal", s.InheritedID)
	case s.Recorded && s.Session == nil:
		return `recorded into a session that was deleted; "carrier attach <session>" switches it`
	case s.OldHooks:
		return fmt.Sprintf("recording session %d, but started by a carrier older than 1.1.0: its prompt cannot follow switches; open a new terminal", s.Session.ID)
	case s.Recorded:
		return "recording session " + sessionName(s.Session)
	default:
		return "not a recorded shell"
	}
}

// tmuxEnvironment checks, inside tmux, that the tmux server does not hold the
// variables of a recorded shell started by a carrier older than 1.2.2. New
// panes would inherit them: they would not get their own session, and zsh
// would look for its config in that shell's hook directory, which is removed
// when that shell exits. applies is false outside tmux.
func tmuxEnvironment() (ok bool, detail string, applies bool) {
	if os.Getenv("TMUX") == "" || !commandAvailable("tmux") {
		return true, "", false
	}
	out, err := exec.Command("tmux", "show-environment", "-g").Output()
	if err != nil {
		return true, "", false
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, found := strings.Cut(line, "="); found {
			env[k] = v
		}
	}
	_, hasSession := env["CARRIER_SESSION_ID"]
	_, hasTTY := env["CARRIER_TTY"]
	staleZDOTDIR := strings.HasPrefix(filepath.Base(env["ZDOTDIR"]), "carrier-shell-")
	if (hasSession && !hasTTY) || staleZDOTDIR {
		return false, `the tmux server holds the variables of a shell started by a carrier older than 1.2.2, so new panes are not recorded properly; run "tmux kill-server" and start tmux again`, true
	}
	return true, "new panes get their own session", true
}

func check(w io.Writer, t theme, name string, ok bool, detail string, err error) {
	statusStyle := t.Success
	status := "ok"
	if !ok {
		status = "warn"
		statusStyle = t.Warning
	}
	if err != nil {
		detail = err.Error()
		statusStyle = t.Danger
	}
	_, _ = fmt.Fprintf(
		w, "%s %s  %s\n",
		t.Bold.Render(padRight(name, 12)),
		statusStyle.Render(status),
		t.Muted.Render(detail),
	)
}

func dirWritable(path string) bool {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(path, ".carrier-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return false
	}
	return os.Remove(name) == nil
}

func fileParentWritable(path string) bool {
	return dirWritable(filepath.Dir(path))
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func shellProgram(configured string) string {
	if configured != "" {
		return configured
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "(not set)"
}

func shellSupported(configured string) bool {
	name := filepath.Base(shellProgram(configured))
	return strings.Contains(name, "zsh") || strings.Contains(name, "bash") || strings.Contains(name, "fish")
}

// shellConfigFiles lists the config files that load "carrier init".
func shellConfigFiles(home string) map[string]string {
	zdot := os.Getenv("ZDOTDIR")
	if zdot == "" {
		zdot = home
	}
	return map[string]string{
		"zsh":  filepath.Join(zdot, ".zshrc"),
		"bash": filepath.Join(home, ".bashrc"),
		"fish": filepath.Join(home, ".config", "fish", "config.fish"),
	}
}

// autoSessionStatus reports which shell configs run "carrier init", or how to
// turn automatic sessions on for the current shell.
func autoSessionStatus(configured, retention string) string {
	home, _ := os.UserHomeDir()
	files := shellConfigFiles(home)
	var enabled []string
	for _, name := range carriershell.InitShells {
		if b, err := os.ReadFile(files[name]); err == nil && strings.Contains(string(b), "carrier init") {
			enabled = append(enabled, collapseHome(files[name]))
		}
	}
	if len(enabled) > 0 {
		return fmt.Sprintf("on in %s (kept %s)", strings.Join(enabled, ", "), retention)
	}
	name := filepath.Base(shellProgram(configured))
	if file, ok := files[name]; ok {
		return fmt.Sprintf("off; add %s to %s", carriershell.InitLine(name), collapseHome(file))
	}
	return "off; see carrier init"
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
