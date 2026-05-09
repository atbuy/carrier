package shell

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/user/carrier/internal/config"
	"github.com/user/carrier/internal/logs"
)

func Run(cfg config.Config, notify, notifyAlways, noRedact bool) error {
	program := cfg.Shell.Program
	if program == "" {
		program = os.Getenv("SHELL")
	}
	if program == "" {
		program = "/bin/zsh"
	}
	carrierPath, err := os.Executable()
	if err != nil {
		return err
	}
	statePath := filepath.Join(os.TempDir(), "carrier-shell-"+strconvPID()+".json")
	_ = os.WriteFile(statePath, []byte(`{}`), 0o600)
	hookDir, err := WriteHookDir(carrierPath, statePath, program)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	defer func() { _ = os.Remove(statePath) }()

	cmd := exec.Command(program, "-i")
	cmd.Env = append(
		os.Environ(),
		"ZDOTDIR="+hookDir,
		"CARRIER_SHELL_STATE="+statePath,
		boolEnv("CARRIER_NOTIFY", notify),
		boolEnv("CARRIER_NOTIFY_ALWAYS", notifyAlways),
		boolEnv("CARRIER_NO_REDACT", noRedact),
	)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()
	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
	}
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	redactor := logs.NewRedactor(cfg.Redaction.Enabled && !noRedact, cfg.Redaction.Patterns)
	state := &StateFile{Path: statePath}
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = os.Stdout.Write(chunk)
			cur := state.Read()
			if cur.CurrentLog != "" {
				if f, ferr := os.OpenFile(cur.CurrentLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); ferr == nil {
					_, _ = f.Write(redactor.Redact(chunk))
					_ = f.Close()
				}
			}
		}
		if err != nil {
			break
		}
	}
	return cmd.Wait()
}

func boolEnv(k string, v bool) string {
	if v {
		return k + "=1"
	}
	return k + "=0"
}

func strconvPID() string {
	return strings.TrimSpace(strconv.Itoa(os.Getpid()))
}
