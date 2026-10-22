package shell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/logs"
)

// RunEndFunc is called after a command's run has ended and its terminal log
// has been flushed, so the log is complete. A non-empty return value is
// printed to the terminal as its own line before further shell output.
type RunEndFunc func(runID int64) string

// Options configures a tracked shell started by Run.
type Options struct {
	Notify       bool
	NotifyAlways bool
	NoRedact     bool
	SessionID    int64
	Label        string
	// Quiet skips the startup banner, for shells started automatically from
	// a shell config.
	Quiet bool
	// OnRunEnd may be nil.
	OnRunEnd RunEndFunc
}

// Run starts a tracked shell and blocks until it exits. It returns the last
// hook state, whose CurrentID is non-zero when a command was still in flight
// as the shell exited (e.g. "exit" itself), so the caller can finish that run.
func Run(cfg config.Config, opts Options) (State, error) {
	sessionID, label, onRunEnd := opts.SessionID, opts.Label, opts.OnRunEnd
	program := cfg.Shell.Program
	if program == "" {
		program = os.Getenv("SHELL")
	}
	if program == "" {
		program = "/bin/zsh"
	}
	carrierPath, err := os.Executable()
	if err != nil {
		return State{}, err
	}
	statePath := filepath.Join(os.TempDir(), "carrier-shell-"+strconvPID()+".json")
	initState, _ := json.Marshal(State{SessionID: sessionID})
	_ = os.WriteFile(statePath, initState, 0o600)
	_ = WriteSessionInfo(statePath, sessionID, label)
	hookDir, err := WriteHookDir(carrierPath, statePath, program, sessionID, label, cfg.UI.Theme.Label, !opts.Quiet)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	defer func() { _ = os.Remove(statePath) }()
	defer func() { _ = os.Remove(SessionInfoPath(statePath)) }()

	cmd := exec.Command(program, shellArgs(program, hookDir)...)
	env := os.Environ()
	if os.Getenv("TERM") == "" {
		env = append(env, "TERM=xterm-256color")
	}
	if os.Getenv("COLORTERM") == "" {
		env = append(env, "COLORTERM=truecolor")
	}
	cmd.Env = append(
		env,
		"ZDOTDIR="+hookDir,
		"CARRIER_SHELL_STATE="+statePath,
		fmt.Sprintf("CARRIER_SESSION_ID=%d", sessionID),
		"CARRIER_SESSION_LABEL="+label,
		boolEnv("CARRIER_NOTIFY", opts.Notify),
		boolEnv("CARRIER_NOTIFY_ALWAYS", opts.NotifyAlways),
		boolEnv("CARRIER_NO_REDACT", opts.NoRedact),
		// The exit hook asks for the session's outcome only when this is set.
		boolEnv("CARRIER_ASK_OUTCOME", cfg.Shell.AskOutcome),
	)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return State{}, err
	}
	defer func() { _ = ptmx.Close() }()

	if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}

	stopResize := watchResize(ptmx)
	defer stopResize()

	if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
	}
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	redactor := logs.NewRedactorWithBuiltins(cfg.Redaction.Enabled && !opts.NoRedact, cfg.Redaction.Patterns)
	maxOutputBytes := logs.MaxOutputBytes(cfg.Storage.MaxOutputMB)
	state := &StateFile{Path: statePath}
	logWriter := &sessionLogWriter{redactor: redactor, maxOutputBytes: maxOutputBytes}
	defer func() { _ = logWriter.Close() }()
	buf := make([]byte, 32*1024)
	var prevID int64
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			cur := state.Read()
			if prevID != 0 && cur.CurrentID != prevID {
				// The hooks have ended prevID. Flush its log (the redacting
				// writer holds back its tail) before the caller reads it, and
				// print any message ahead of the prompt that follows.
				_ = logWriter.Close()
				if onRunEnd != nil {
					if msg := onRunEnd(prevID); msg != "" {
						_, _ = io.WriteString(os.Stdout, "\r"+msg+"\r\n")
					}
				}
			}
			prevID = cur.CurrentID
			_, _ = os.Stdout.Write(chunk)
			if cur.CurrentLog != "" {
				_, _ = logWriter.Write(cur.CurrentLog, chunk)
			}
		}
		if err != nil {
			break
		}
	}
	waitErr := cmd.Wait()
	return state.Read(), waitErr
}

func shellArgs(program, hookDir string) []string {
	name := filepath.Base(program)
	switch {
	case strings.Contains(name, "bash"):
		return []string{"--rcfile", filepath.Join(hookDir, ".bashrc"), "-i"}
	case strings.Contains(name, "fish"):
		return []string{"--init-command", "source " + filepath.Join(hookDir, "carrier.fish")}
	default:
		return []string{"-i"}
	}
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

// altScreenEnter / altScreenLeave are the DEC private-mode sequences that
// TUIs use to enter and exit the alternate screen buffer. We suppress log
// writes while in alt-screen so that replaying the log (carrier show) does
// not re-render TUI frames or corrupt the viewer's terminal.
var (
	altScreenEnter = []byte("\x1b[?1049h")
	altScreenLeave = []byte("\x1b[?1049l")
)

type sessionLogWriter struct {
	path           string
	file           *os.File
	writer         *logs.RedactingWriter
	redactor       logs.Redactor
	maxOutputBytes int64
	inAltScreen    bool
}

func (w *sessionLogWriter) Write(path string, chunk []byte) (int, error) {
	total := len(chunk)
	// Scan for alt-screen transitions and split/suppress accordingly.
	for len(chunk) > 0 {
		if !w.inAltScreen {
			idx := bytes.Index(chunk, altScreenEnter)
			if idx < 0 {
				// No transition — write everything.
				if err := w.ensure(path); err != nil {
					return 0, err
				}
				if _, err := w.writer.Write(chunk); err != nil {
					return 0, err
				}
				break
			}
			// Write bytes before the enter sequence, then suppress.
			if idx > 0 {
				if err := w.ensure(path); err != nil {
					return 0, err
				}
				if _, err := w.writer.Write(chunk[:idx]); err != nil {
					return 0, err
				}
			}
			w.inAltScreen = true
			chunk = chunk[idx+len(altScreenEnter):]
		} else {
			idx := bytes.Index(chunk, altScreenLeave)
			if idx < 0 {
				// Still in TUI — discard everything.
				break
			}
			w.inAltScreen = false
			chunk = chunk[idx+len(altScreenLeave):]
		}
	}
	return total, nil
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
