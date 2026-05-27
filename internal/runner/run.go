package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/atbuy/carrier/internal/command"
	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
	"github.com/atbuy/carrier/internal/logs"
	"github.com/atbuy/carrier/internal/notify"
	"github.com/atbuy/carrier/internal/store"
)

type Options struct {
	Mode         string
	Argv         []string
	CWD          string
	Notify       bool
	NotifyAlways bool
	NoRedact     bool
	Quiet        bool
	Timeout      time.Duration
	Ctx          context.Context
}

func Run(cfg config.Config, st *store.Store, opts Options) (int, error) {
	if len(opts.Argv) == 0 {
		return 1, errors.New("missing command")
	}
	if opts.CWD == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return 1, err
		}
		opts.CWD = cwd
	}
	started := time.Now()
	host, _ := os.Hostname()
	shell := os.Getenv("SHELL")
	argvJSON, _ := json.Marshal(opts.Argv)

	// Collect git metadata and capture env concurrently with child startup.
	gitCh := make(chan gitmeta.Meta, 1)
	envCh := make(chan *int64, 1)
	go func() { gitCh <- gitmeta.Collect(opts.CWD) }()
	go func() {
		var envID *int64
		if envJSON := captureEnv(cfg); envJSON != "" {
			if eid, err := st.InsertOrGetEnvironment(envJSON); err == nil {
				envID = &eid
			}
		}
		envCh <- envID
	}()

	// Create a minimal run record immediately to obtain an ID for log paths.
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: opts.Mode, Command: command.Display(opts.Argv),
		ArgvJSON: string(argvJSON), CWD: opts.CWD, StartedAt: started, Hostname: host, Shell: shell,
		NotifyRequested: opts.Notify, NotifyAlways: opts.NotifyAlways,
	})
	if err != nil {
		return 1, err
	}
	stdoutPath := logs.StdoutPath(cfg.Storage.DataDir, id)
	stderrPath := logs.StderrPath(cfg.Storage.DataDir, id)
	if err := st.UpdatePaths(id, stdoutPath, stderrPath, ""); err != nil {
		return 1, err
	}
	if !opts.Quiet {
		_, _ = io.WriteString(os.Stderr, runStartedLine(os.Stderr, id, cfg.UI.Theme))
	}

	// Run the child while git/env goroutines execute in parallel.
	exit, killed, finishErr := execute(opts, cfg, stdoutPath, stderrPath)

	// Drain goroutines (done long before any non-trivial child finishes).
	git := <-gitCh
	envID := <-envCh

	status := store.StatusSuccess
	switch {
	case killed:
		status = store.StatusKilled
	case exit != 0:
		status = store.StatusFailed
	}
	finished := time.Now()
	// Single UPDATE backfills git/env and records the terminal status.
	if err := st.FinalizeRun(id, store.FinalizeParams{
		GitRoot: git.Root, GitBranch: git.Branch, GitCommit: git.Commit, GitDirty: git.Dirty,
		EnvID: envID, Status: status, ExitCode: exit, Started: started, Finished: finished,
	}); err != nil && finishErr == nil {
		finishErr = err
	}
	if opts.Notify || opts.NotifyAlways {
		if r, err := st.GetRun(id); err == nil {
			if notifyErr := notify.MaybeSend(cfg, *r); notifyErr != nil && !opts.Quiet {
				_, _ = fmt.Fprintf(os.Stderr, "carrier: notify: %s\n", notifyErr)
			}
		}
	}
	return exit, finishErr
}

func execute(opts Options, cfg config.Config, stdoutPath, stderrPath string) (int, bool, error) {
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0o755); err != nil {
		return 1, false, err
	}
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return 1, false, err
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return 1, false, err
	}
	defer func() { _ = stderrFile.Close() }()

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	argv := shellFallback(opts.Argv)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = opts.CWD
	cmd.Stdin = os.Stdin
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, false, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 1, false, err
	}
	redactor := logs.NewRedactorWithBuiltins(cfg.Redaction.Enabled && !opts.NoRedact, cfg.Redaction.Patterns)
	maxOutputBytes := logs.MaxOutputBytes(cfg.Storage.MaxOutputMB)
	stdoutLog := logs.NewRedactingWriter(logs.NewCappedWriter(stdoutFile, maxOutputBytes), redactor)
	stderrLog := logs.NewRedactingWriter(logs.NewCappedWriter(stderrFile, maxOutputBytes), redactor)
	defer func() { _ = stdoutLog.Close() }()
	defer func() { _ = stderrLog.Close() }()
	stdoutDst := io.MultiWriter(os.Stdout, stdoutLog)
	stderrDst := io.MultiWriter(os.Stderr, stderrLog)
	if err := cmd.Start(); err != nil {
		return 127, false, startError(opts.Argv[0], err)
	}

	var killed atomic.Bool
	if opts.Timeout > 0 {
		timer := time.AfterFunc(opts.Timeout, func() {
			killed.Store(true)
			_ = cmd.Process.Signal(os.Interrupt)
			time.Sleep(5 * time.Second)
			_ = cmd.Process.Kill()
		})
		defer timer.Stop()
	}

	outCh := asyncCopy(stdoutDst, stdout)
	errCh := asyncCopy(stderrDst, stderr)
	copyErr := waitCopies(outCh, errCh)
	if err := stdoutLog.Close(); err != nil && copyErr == nil {
		copyErr = err
	}
	if err := stderrLog.Close(); err != nil && copyErr == nil {
		copyErr = err
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		killed.Store(true)
	}
	exit, _ := ExitCode(waitErr)
	if copyErr != nil {
		return exit, killed.Load(), copyErr
	}
	return exit, killed.Load(), nil
}

func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func startError(name string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("command not found: %s", name)
	}
	return fmt.Errorf("start command %s: %w", command.Quote(name), err)
}

// captureEnv serialises os.Environ() as a JSON object with values redacted.
// Redaction is applied before storage so secrets never reach disk.
// Returns "" when capture is disabled.
func captureEnv(cfg config.Config) string {
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

// shellFallback returns argv unchanged when argv[0] is a known executable.
// When argv[0] cannot be found in PATH, it wraps the command in the user's
// shell with -i -c so that aliases and shell functions are expanded.
func shellFallback(argv []string) []string {
	if _, err := exec.LookPath(argv[0]); err == nil {
		return argv
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell, "-i", "-c", command.Display(argv)}
}
