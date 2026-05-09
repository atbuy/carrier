package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/version"
)

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "check local carrier setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDoctor()
		},
	}
}

func (a *app) runDoctor() error {
	check("version", true, version.Current().Version, nil)
	configPath, err := config.Path()
	check("config path", err == nil, configPath, err)
	check("data dir", dirWritable(a.cfg.Storage.DataDir), a.cfg.Storage.DataDir, nil)
	check("runs dir", dirWritable(filepath.Join(a.cfg.Storage.DataDir, "runs")), filepath.Join(a.cfg.Storage.DataDir, "runs"), nil)
	check("sqlite db", fileParentWritable(filepath.Join(a.cfg.Storage.DataDir, "carrier.db")), filepath.Join(a.cfg.Storage.DataDir, "carrier.db"), nil)
	check("git", commandAvailable("git"), "git executable available", nil)
	check("notify-send", commandAvailable("notify-send"), "optional desktop notifications", nil)
	check("shell", shellSupported(a.cfg.Shell.Program), shellProgram(a.cfg.Shell.Program), nil)
	check("terminal", term.IsTerminal(int(os.Stdout.Fd())), "stdout is a TTY", nil)
	return nil
}

func check(name string, ok bool, detail string, err error) {
	status := "ok"
	if !ok {
		status = "warn"
	}
	if err != nil {
		detail = err.Error()
	}
	fmt.Printf("%-12s %s  %s\n", name, status, detail)
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
