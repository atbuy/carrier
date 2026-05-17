package shell

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/logs"
)

func Run(cfg config.Config, notify, notifyAlways, noRedact bool) error {
	warnShellAlphaOnce(cfg.Storage.DataDir)
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

	cmd := exec.Command(program, shellArgs(program, hookDir)...)
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

	if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		for range sigwinch {
			if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
			}
		}
	}()
	defer func() {
		signal.Stop(sigwinch)
		close(sigwinch)
	}()

	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
	}
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	redactor := logs.NewRedactor(cfg.Redaction.Enabled && !noRedact, cfg.Redaction.Patterns)
	maxOutputBytes := logs.MaxOutputBytes(cfg.Storage.MaxOutputMB)
	state := &StateFile{Path: statePath}
	logWriter := &sessionLogWriter{redactor: redactor, maxOutputBytes: maxOutputBytes}
	defer func() { _ = logWriter.Close() }()
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = os.Stdout.Write(chunk)
			cur := state.Read()
			if cur.CurrentLog != "" {
				_, _ = logWriter.Write(cur.CurrentLog, chunk)
			}
		}
		if err != nil {
			break
		}
	}
	return cmd.Wait()
}

func shellArgs(program, hookDir string) []string {
	name := filepath.Base(program)
	if strings.Contains(name, "bash") {
		return []string{"--rcfile", filepath.Join(hookDir, ".bashrc"), "-i"}
	}
	return []string{"-i"}
}

func warnShellAlphaOnce(dataDir string) {
	if dataDir == "" {
		return
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dataDir, "shell-alpha-warning")
	if _, err := os.Stat(path); err == nil {
		return
	}
	_, _ = os.Stderr.WriteString("carrier: shell mode is alpha; use carrier run for precise capture\n")
	_ = os.WriteFile(path, []byte("shown\n"), 0o600)
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

type sessionLogWriter struct {
	path           string
	file           *os.File
	writer         *logs.RedactingWriter
	redactor       logs.Redactor
	maxOutputBytes int64
}

func (w *sessionLogWriter) Write(path string, chunk []byte) (int, error) {
	if err := w.ensure(path); err != nil {
		return 0, err
	}
	return w.writer.Write(chunk)
}

func (w *sessionLogWriter) ensure(path string) error {
	if w.path == path && w.writer != nil {
		return nil
	}
	if err := w.Close(); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	var alreadyWritten int64
	if info, serr := f.Stat(); serr == nil {
		alreadyWritten = info.Size()
	}
	w.path = path
	w.file = f
	w.writer = logs.NewRedactingWriter(logs.NewCappedAppendWriter(f, w.maxOutputBytes, alreadyWritten), w.redactor)
	return nil
}

func (w *sessionLogWriter) Close() error {
	var first error
	if w.writer != nil {
		if err := w.writer.Close(); err != nil {
			first = err
		}
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil && first == nil {
			first = err
		}
	}
	w.path = ""
	w.file = nil
	w.writer = nil
	return first
}
