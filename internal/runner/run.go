package runner

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/user/carrier/internal/config"
	"github.com/user/carrier/internal/gitmeta"
	"github.com/user/carrier/internal/logs"
	"github.com/user/carrier/internal/notify"
	"github.com/user/carrier/internal/store"
)

type Options struct {
	Mode         string
	Argv         []string
	CWD          string
	Notify       bool
	NotifyAlways bool
	NoRedact     bool
	Quiet        bool
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
	git := gitmeta.Collect(opts.CWD)
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: opts.Mode, Command: strings.Join(opts.Argv, " "),
		ArgvJSON: string(argvJSON), CWD: opts.CWD, StartedAt: started, Hostname: host, Shell: shell,
		GitRoot: git.Root, GitBranch: git.Branch, GitCommit: git.Commit, GitDirty: git.Dirty,
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
		_, _ = os.Stderr.WriteString("carrier: run " + strconvID(id) + "\n")
	}
	exit, finishErr := execute(opts, cfg, stdoutPath, stderrPath)
	status := store.StatusSuccess
	if exit != 0 {
		status = store.StatusFailed
	}
	finished := time.Now()
	if err := st.FinishRun(id, status, exit, finished); err != nil && finishErr == nil {
		finishErr = err
	}
	r, err := st.GetRun(id)
	if err == nil {
		notify.MaybeSend(cfg, *r)
	}
	return exit, finishErr
}

func execute(opts Options, cfg config.Config, stdoutPath, stderrPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0o755); err != nil {
		return 1, err
	}
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return 1, err
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return 1, err
	}
	defer func() { _ = stderrFile.Close() }()

	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...)
	cmd.Dir = opts.CWD
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 1, err
	}
	redactor := logs.NewRedactor(cfg.Redaction.Enabled && !opts.NoRedact, cfg.Redaction.Patterns)
	stdoutDst := io.MultiWriter(os.Stdout, logs.NewRedactingWriter(stdoutFile, redactor))
	stderrDst := io.MultiWriter(os.Stderr, logs.NewRedactingWriter(stderrFile, redactor))
	if err := cmd.Start(); err != nil {
		return 127, err
	}
	outCh := asyncCopy(stdoutDst, stdout)
	errCh := asyncCopy(stderrDst, stderr)
	waitErr := cmd.Wait()
	copyErr := waitCopies(outCh, errCh)
	exit, _ := ExitCode(waitErr)
	if copyErr != nil {
		return exit, copyErr
	}
	return exit, nil
}

func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}
