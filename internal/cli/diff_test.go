package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestDiffStreamPath(t *testing.T) {
	run := &store.Run{
		StdoutPath:         "/tmp/stdout.log",
		StderrPath:         "/tmp/stderr.log",
		TerminalOutputPath: "",
	}

	// stdout
	p, err := diffStreamPath(run, "stdout")
	if err != nil || p != "/tmp/stdout.log" {
		t.Errorf("stdout: got %q %v", p, err)
	}

	// stderr
	p, err = diffStreamPath(run, "stderr")
	if err != nil || p != "/tmp/stderr.log" {
		t.Errorf("stderr: got %q %v", p, err)
	}

	// terminal — no PTY log → error
	_, err = diffStreamPath(run, "terminal")
	if err == nil {
		t.Error("terminal with no PTY path: expected error")
	}

	// terminal — with PTY log
	run.TerminalOutputPath = "/tmp/pty.log"
	p, err = diffStreamPath(run, "terminal")
	if err != nil || p != "/tmp/pty.log" {
		t.Errorf("terminal: got %q %v", p, err)
	}

	// auto — prefers terminal when present
	p, err = diffStreamPath(run, "auto")
	if err != nil || p != "/tmp/pty.log" {
		t.Errorf("auto with PTY: got %q %v", p, err)
	}

	// auto — falls back to stdout when no PTY
	run.TerminalOutputPath = ""
	p, err = diffStreamPath(run, "auto")
	if err != nil || p != "/tmp/stdout.log" {
		t.Errorf("auto without PTY: got %q %v", p, err)
	}
}

func TestWriteDiffTmp(t *testing.T) {
	content := "line one\nline two\n"
	path, err := writeDiffTmp(42, content)
	if err != nil {
		t.Fatalf("writeDiffTmp: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tmp file: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
	if !strings.Contains(path, "carrier-diff-42-") {
		t.Errorf("path %q missing expected prefix", path)
	}
}

func TestDiffCmdMetadata(t *testing.T) {
	cmd := (&app{}).diffCmd()
	if cmd.Use != "diff <id1> <id2>" {
		t.Errorf("Use = %q", cmd.Use)
	}
}

func TestDiffCmdBadIDs(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.diffCmd()

	if err := cmd.RunE(cmd, []string{"abc", "2"}); err == nil {
		t.Error("expected error for non-integer id1")
	}
	if err := cmd.RunE(cmd, []string{"1", "xyz"}); err == nil {
		t.Error("expected error for non-integer id2")
	}
}

func TestDiffCmdMissingRuns(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.diffCmd()

	if err := cmd.RunE(cmd, []string{"1", "2"}); err == nil {
		t.Error("expected error for missing run IDs")
	}
}

func TestDiffCmdSameOutput(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	makeRun := func(content string) int64 {
		id, err := st.CreateRun(store.CreateRun{
			Status:    store.StatusSuccess,
			Mode:      store.ModeRun,
			Command:   "echo hi",
			ArgvJSON:  `["echo","hi"]`,
			CWD:       "/tmp",
			StartedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		f, err := os.CreateTemp(dir, "stdout-*.log")
		if err != nil {
			t.Fatalf("create temp: %v", err)
		}
		_, _ = f.WriteString(content)
		_ = f.Close()
		if err := st.UpdatePaths(id, f.Name(), "", ""); err != nil {
			t.Fatalf("update paths: %v", err)
		}
		return id
	}

	id1 := makeRun("hello\n")
	id2 := makeRun("hello\n")

	a := &app{st: st}
	cmd := a.diffCmd()

	// identical outputs — diff exits 0, RunE returns nil
	if err := cmd.RunE(cmd, []string{
		fmt.Sprintf("%d", id1),
		fmt.Sprintf("%d", id2),
	}); err != nil {
		t.Fatalf("diff of identical runs: %v", err)
	}
}
