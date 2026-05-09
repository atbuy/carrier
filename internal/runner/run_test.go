package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

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
