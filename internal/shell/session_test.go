package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
)

func TestRunExecutesProgramAndWritesCurrentLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	program := filepath.Join(dir, "carrier-shell-test")
	script := `#!/bin/sh
printf '{"current_id":1,"current_log":"%s"}' "$TEST_LOG_PATH" > "$CARRIER_SHELL_STATE"
printf 'TOKEN=secret\n'
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}

	logPath := filepath.Join(dir, "session.log")
	t.Setenv("TEST_LOG_PATH", logPath)

	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Storage.MaxOutputMB = 1
	cfg.Shell.Program = program
	cfg.Redaction.Enabled = true
	cfg.Redaction.Patterns = []string{`TOKEN=\S+`}

	if err := Run(cfg, false, false, false, 0); err != nil {
		t.Fatalf("Run: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("log missing redacted output: %q", got)
	}
	if strings.Contains(got, "TOKEN=secret") {
		t.Fatalf("secret leaked in session log: %q", got)
	}

	if _, err := os.Stat(filepath.Join(cfg.Storage.DataDir, "shell-alpha-warning")); err != nil {
		t.Fatalf("warning marker missing: %v", err)
	}
}

func TestWarnShellAlphaOnceNoDataDir(t *testing.T) {
	warnShellAlphaOnce("")
}

func TestWarnShellAlphaOnceIgnoresMkdirError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	warnShellAlphaOnce(filepath.Join(filePath, "child"))
}

func TestStateFileReadWrite(t *testing.T) {
	sf := &StateFile{Path: filepath.Join(t.TempDir(), "state.json")}

	if got := sf.Read(); got.CurrentID != 0 || got.CurrentLog != "" {
		t.Fatalf("missing state should read zero value: %#v", got)
	}

	want := State{CurrentID: 42, CurrentLog: "/tmp/run.log"}
	if err := sf.Write(want); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if got := sf.Read(); got != want {
		t.Fatalf("state mismatch: got %#v want %#v", got, want)
	}
}

func TestWarnShellAlphaOnceWritesMarker(t *testing.T) {
	dir := t.TempDir()

	warnShellAlphaOnce(dir)
	if _, err := os.Stat(filepath.Join(dir, "shell-alpha-warning")); err != nil {
		t.Fatalf("warning marker missing: %v", err)
	}
	warnShellAlphaOnce(dir)
}

func TestBoolEnv(t *testing.T) {
	if got := boolEnv("MYVAR", true); got != "MYVAR=1" {
		t.Fatalf("boolEnv true = %q", got)
	}
	if got := boolEnv("MYVAR", false); got != "MYVAR=0" {
		t.Fatalf("boolEnv false = %q", got)
	}
}

func TestStrconvPID(t *testing.T) {
	got := strconvPID()
	if got == "" {
		t.Fatal("strconvPID returned empty string")
	}
	if strings.ContainsAny(got, " \t\n") {
		t.Fatalf("strconvPID has whitespace: %q", got)
	}
}

func TestSessionLogWriterEnsureAndClose(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "log1.txt")
	path2 := filepath.Join(dir, "log2.txt")

	w := &sessionLogWriter{
		maxOutputBytes: 1024 * 1024,
	}

	// Write to first path.
	n, err := w.Write(path1, []byte("hello "))
	if err != nil {
		t.Fatalf("write path1: %v", err)
	}
	if n != 6 {
		t.Fatalf("n = %d", n)
	}

	// Write to same path again — exercises the "same path reuse" branch in ensure.
	if _, err := w.Write(path1, []byte("world")); err != nil {
		t.Fatalf("write path1 again: %v", err)
	}

	// Write to second path (triggers close of first, open of second).
	if _, err := w.Write(path2, []byte("new")); err != nil {
		t.Fatalf("write path2: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Verify files.
	b1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read path1: %v", err)
	}
	if string(b1) != "hello world" {
		t.Fatalf("path1 = %q", b1)
	}

	b2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read path2: %v", err)
	}
	if string(b2) != "new" {
		t.Fatalf("path2 = %q", b2)
	}
}

func TestSessionLogWriterWriteToInvalidPath(t *testing.T) {
	w := &sessionLogWriter{maxOutputBytes: 1024}
	// Write to a non-existent directory → ensure fails.
	_, err := w.Write("/does/not/exist/log.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error writing to invalid path")
	}
}

func TestSessionLogWriterIdempotentClose(t *testing.T) {
	w := &sessionLogWriter{maxOutputBytes: 1024}
	// Close on zero-value writer should be safe.
	if err := w.Close(); err != nil {
		t.Fatalf("close zero writer: %v", err)
	}
}

func TestShellArgsUsesBashRcFile(t *testing.T) {
	dir := t.TempDir()

	got := shellArgs("/bin/bash", dir)
	want := []string{"--rcfile", filepath.Join(dir, ".bashrc"), "-i"}
	if len(got) != len(want) {
		t.Fatalf("shell args length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shell arg %d = %q, want %q", i, got[i], want[i])
		}
	}
	if got := shellArgs("/bin/zsh", dir); len(got) != 1 || got[0] != "-i" {
		t.Fatalf("zsh args = %#v", got)
	}
}
