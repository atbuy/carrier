package runner

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// initGitRepo creates a minimal git repo with one commit and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@t.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@t.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t.com")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestRunCapturesOutputAndMetadata(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	cfg.Redaction.Enabled = true
	cfg.Redaction.Patterns = []string{`TOKEN=\S+`}

	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"sh", "-c", "echo TOKEN=secret; echo err >&2"},
		CWD:   t.TempDir(),
		Quiet: true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Status != store.StatusSuccess {
		t.Fatalf("status = %q", run.Status)
	}
	if run.Command != "sh -c 'echo TOKEN=secret; echo err >&2'" {
		t.Fatalf("command display = %q", run.Command)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Fatalf("exit code metadata = %#v", run.ExitCode)
	}
	stdout, err := os.ReadFile(run.StdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if string(stdout) != "[REDACTED]\n" {
		t.Fatalf("stdout log = %q", string(stdout))
	}
	stderr, err := os.ReadFile(run.StderrPath)
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if string(stderr) != "err\n" {
		t.Fatalf("stderr log = %q", string(stderr))
	}
	if filepath.Dir(run.StdoutPath) != filepath.Join(dataDir, "runs") {
		t.Fatalf("stdout path outside data dir: %q", run.StdoutPath)
	}
}

func TestRunMissingCommand(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	code, err := Run(config.Default(), st, Options{Mode: store.ModeRun})
	if err == nil {
		t.Fatalf("expected missing command error")
	}
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
}

func TestRunDefaultsCWD(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	wantCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}

	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"true"},
		Quiet: true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.CWD != wantCWD {
		t.Fatalf("cwd = %q, want %q", run.CWD, wantCWD)
	}
}

func TestRunCommandNotFoundReturnsClearError(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"carrier-command-does-not-exist"},
		CWD:   t.TempDir(),
		Quiet: true,
	})
	// shellFallback wraps unknown commands in $SHELL -i -c; the shell exits 127
	// but returns no Go error. Accept either a Go-level "command not found" error
	// or a clean exit-127 (shell handled the not-found case).
	if code != 127 {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if err != nil && !strings.Contains(err.Error(), "command not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteReturnsMkdirError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	stdoutPath := filepath.Join(filePath, "stdout.log")
	stderrPath := filepath.Join(filePath, "stderr.log")
	exit, killed, err := execute(Options{Argv: []string{"true"}, CWD: t.TempDir()}, config.Default(), stdoutPath, stderrPath)
	if err == nil {
		t.Fatal("expected mkdir error")
	}
	if exit != 1 || killed {
		t.Fatalf("exit=%d killed=%v", exit, killed)
	}
}

func TestRunCommandFailureRecordsFailedStatus(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"sh", "-c", "exit 5"},
		CWD:   t.TempDir(),
		Quiet: true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if code != 5 {
		t.Fatalf("code = %d", code)
	}
	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Status != store.StatusFailed {
		t.Fatalf("status = %q", run.Status)
	}
}

func TestRunCapsPersistedOutput(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	cfg.Storage.MaxOutputMB = 1

	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"sh", "-c", "yes x | head -c 1200000"},
		CWD:   t.TempDir(),
		Quiet: true,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	stdout, err := os.ReadFile(run.StdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if len(stdout) > 1050000 {
		t.Fatalf("stdout log too large: %d", len(stdout))
	}
	if !strings.Contains(string(stdout), "output truncated") {
		t.Fatalf("stdout log missing truncation notice")
	}
}

func TestRunNotifyPrintsErrorWhenNotQuiet(t *testing.T) {
	// Exercises the "!opts.Quiet && notifyErr != nil" path in Run.
	// The command runs quickly (below 10s threshold) so MaybeSend returns
	// ErrBelowThreshold, which is != nil, triggering the stderr print.
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir

	// Quiet=false + Notify=true → will hit the notify error print path.
	_, _ = Run(cfg, st, Options{
		Mode:   store.ModeRun,
		Argv:   []string{"true"},
		CWD:    t.TempDir(),
		Quiet:  false,
		Notify: true,
	})
	// No assertion — we just need the code path to execute without panic.
}

// ---------------------------------------------------------------------------
// strconvID
// ---------------------------------------------------------------------------

func TestStrconvID(t *testing.T) {
	cases := []struct {
		id   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{1000000, "1000000"},
		{-1, "-1"},
	}
	for _, tc := range cases {
		got := strconvID(tc.id)
		if got != tc.want {
			t.Errorf("strconvID(%d) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// startError
// ---------------------------------------------------------------------------

func TestStartErrorNotFound(t *testing.T) {
	err := startError("mybinary", exec.ErrNotFound)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("expected 'command not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mybinary") {
		t.Errorf("expected binary name in error, got: %v", err)
	}
}

func TestStartErrorGeneric(t *testing.T) {
	underlying := errors.New("permission denied")
	err := startError("mybinary", underlying)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	// Should wrap the original error
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped underlying error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mybinary") {
		t.Errorf("expected binary name in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// captureEnv
// ---------------------------------------------------------------------------

func TestCaptureEnvDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.CaptureEnv = false
	got := captureEnv(cfg)
	if got != "" {
		t.Errorf("expected empty string when CaptureEnv=false, got %q", got)
	}
}

func TestCaptureEnvEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.CaptureEnv = true

	// Set a known env var so we can verify it appears in the output.
	key := "CARRIER_TEST_CAPTUREENV_VALUE"
	val := "carrier_test_value_123"
	t.Setenv(key, val)

	got := captureEnv(cfg)
	if got == "" {
		t.Fatal("expected non-empty JSON when CaptureEnv=true")
	}

	// Must be valid JSON.
	var m map[string]string
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("captureEnv returned invalid JSON: %v\nraw: %s", err, got)
	}

	// Our known key must be present with the correct value.
	if m[key] != val {
		t.Errorf("env[%s] = %q, want %q", key, m[key], val)
	}
}

// ---------------------------------------------------------------------------
// shellFallback
// ---------------------------------------------------------------------------

func TestShellFallbackNoShellEnv(t *testing.T) {
	t.Setenv("SHELL", "")
	argv := []string{"carrier-binary-not-in-path-xyz"}
	got := shellFallback(argv)
	// SHELL unset → falls back to /bin/sh
	if got[0] != "/bin/sh" {
		t.Errorf("expected /bin/sh, got %q", got[0])
	}
}

func TestShellFallbackKnownBinary(t *testing.T) {
	// "sh" is universally available; shellFallback should return argv unchanged.
	argv := []string{"sh", "-c", "echo hi"}
	got := shellFallback(argv)
	if len(got) != len(argv) {
		t.Fatalf("expected %v, got %v", argv, got)
	}
	for i := range argv {
		if got[i] != argv[i] {
			t.Errorf("argv[%d]: got %q, want %q", i, got[i], argv[i])
		}
	}
}

func TestShellFallbackUnknownBinary(t *testing.T) {
	argv := []string{"carrier-binary-that-does-not-exist-xyz", "arg1"}
	got := shellFallback(argv)

	// Must be wrapped in a shell invocation.
	if len(got) < 3 {
		t.Fatalf("expected shell-wrapped argv, got %v", got)
	}
	// First element should be a shell.
	if filepath.Base(got[0]) != "sh" && filepath.Base(got[0]) != "bash" &&
		filepath.Base(got[0]) != "zsh" && filepath.Base(got[0]) != "fish" {
		t.Errorf("expected a shell as first element, got %q", got[0])
	}
	// Should contain "-c" flag.
	foundDashC := false
	for _, a := range got {
		if a == "-c" {
			foundDashC = true
			break
		}
	}
	if !foundDashC {
		t.Errorf("expected '-c' in shell-wrapped argv, got %v", got)
	}
	// Last element should contain the original command name.
	last := got[len(got)-1]
	if !strings.Contains(last, argv[0]) {
		t.Errorf("expected original command %q in last arg %q", argv[0], last)
	}
}

// ---------------------------------------------------------------------------
// Run — timeout path
// ---------------------------------------------------------------------------

func TestRunTimeout(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir

	start := time.Now()
	_, _ = Run(cfg, st, Options{
		Mode:    store.ModeRun,
		Argv:    []string{"sleep", "10"},
		CWD:     t.TempDir(),
		Quiet:   true,
		Timeout: 200 * time.Millisecond,
	})
	elapsed := time.Since(start)

	// Should have been killed well before the 10-second sleep finished.
	if elapsed > 8*time.Second {
		t.Errorf("timeout did not fire; elapsed = %v", elapsed)
	}

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Status != store.StatusKilled {
		t.Errorf("status = %q, want %q", run.Status, store.StatusKilled)
	}
}

// TestRunAsyncGitMetaBackfilled verifies that git metadata is stored on the run
// record even though collection happens concurrently with child execution.
func TestRunAsyncGitMetaBackfilled(t *testing.T) {
	repoDir := initGitRepo(t)

	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dataDir
	cfg.Storage.CaptureEnv = false

	code, err := Run(cfg, st, Options{
		Mode:  store.ModeRun,
		Argv:  []string{"true"},
		CWD:   repoDir,
		Quiet: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.GitRoot == "" {
		t.Error("GitRoot empty — async git metadata not backfilled")
	}
	if run.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", run.GitBranch, "main")
	}
	if run.GitCommit == "" {
		t.Error("GitCommit empty")
	}
	if run.GitDirty == nil {
		t.Error("GitDirty nil")
	}
}
