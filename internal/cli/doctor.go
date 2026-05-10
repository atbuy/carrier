package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/notify"
	"github.com/atbuy/carrier/internal/version"
)

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "check local carrier setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDoctor(cmd.OutOrStdout())
		},
	}
}

func (a *app) runDoctor(w io.Writer) error {
	c := outputColors(w)
	check(w, c, "version", true, version.Current().Version, nil)
	configPath, err := config.Path()
	check(w, c, "config path", err == nil, configPath, err)
	check(w, c, "data dir", dirWritable(a.cfg.Storage.DataDir), a.cfg.Storage.DataDir, nil)
	check(w, c, "data size", true, formatBytes(dirSize(a.cfg.Storage.DataDir)), nil)
	check(w, c, "runs dir", dirWritable(filepath.Join(a.cfg.Storage.DataDir, "runs")), filepath.Join(a.cfg.Storage.DataDir, "runs"), nil)
	check(w, c, "sqlite db", fileParentWritable(filepath.Join(a.cfg.Storage.DataDir, "carrier.db")), filepath.Join(a.cfg.Storage.DataDir, "carrier.db"), nil)
	if version, err := a.st.MigrationVersion(); err == nil {
		check(w, c, "migration", true, strconv.FormatInt(version, 10), nil)
	} else {
		check(w, c, "migration", false, "", err)
	}
	check(w, c, "max output", true, fmt.Sprintf("%d MB", a.cfg.Storage.MaxOutputMB), nil)
	check(w, c, "redaction", a.cfg.Redaction.Enabled, fmt.Sprintf("%d patterns", len(a.cfg.Redaction.Patterns)), nil)
	check(w, c, "git", commandAvailable("git"), "git executable available", nil)
	check(w, c, "notify", notify.Available(), "optional desktop notifications", nil)
	check(w, c, "shell", shellSupported(a.cfg.Shell.Program), shellProgram(a.cfg.Shell.Program), nil)
	check(w, c, "shell mode", false, "alpha: use carrier run for precise capture", nil)
	check(w, c, "terminal", term.IsTerminal(int(os.Stdout.Fd())), "stdout is a TTY", nil)
	return nil
}

func check(w io.Writer, c helpColors, name string, ok bool, detail string, err error) {
	status := "ok"
	statusColor := colorGreen
	if !ok {
		status = "warn"
		statusColor = colorYellow
	}
	if err != nil {
		detail = err.Error()
		statusColor = colorRed
	}
	_, _ = fmt.Fprintf(
		w, "%s %s  %s\n",
		c.paint(colorBold+colorCyan, padRight(name, 12)),
		c.paint(statusColor, status),
		c.paint(colorGray, detail),
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
	return strings.Contains(name, "zsh") || strings.Contains(name, "bash")
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
