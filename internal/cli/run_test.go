package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func TestRunCmdEmptyArgsReturnsError(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.runCmd()

	// Empty args → cobra.MinimumNArgs error (no subprocess started).
	err = cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestRunCmdParseRunFlagsError(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.runCmd()

	// `-t` with no value → parseRunFlags error.
	err = cmd.RunE(cmd, []string{"-t"})
	if err == nil {
		t.Fatal("expected error for -t with no value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdRunsCommandAndUsesExitProcess(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg, quiet: true}
	cmd := a.runCmd()

	var exitCode int
	oldExit := exitProcess
	exitProcess = func(code int) { exitCode = code }
	t.Cleanup(func() { exitProcess = oldExit })

	if err := cmd.RunE(cmd, []string{"true"}); err != nil {
		t.Fatalf("run command: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Command != "true" || run.Status != store.StatusSuccess {
		t.Fatalf("run = %#v", run)
	}
}

func TestOpenIdempotent(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// a.st is already set → open() should be a no-op.
	a := &app{st: st, cfg: config.Default()}
	if err := a.open(); err != nil {
		t.Fatalf("open with existing store: %v", err)
	}
	if a.st != st {
		t.Fatal("open should not replace existing store")
	}
}

func TestRerunCmdBadID(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.rerunCmd()

	err := cmd.RunE(cmd, []string{"notanid"})
	if err == nil {
		t.Fatal("expected error for bad ID")
	}
}

func TestRerunCmdNotFound(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	a := &app{st: st, cfg: config.Default()}
	cmd := a.rerunCmd()

	err := cmd.RunE(cmd, []string{"99999"})
	if err == nil {
		t.Fatal("expected error for non-existent run")
	}
}

func TestRerunCmdRunsStoredArgvAndUsesExitProcess(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	originalID := createFinishedRun(t, st, store.StatusSuccess, "true", `["true"]`, t.TempDir())
	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg, quiet: true}
	cmd := a.rerunCmd()

	var exitCode int
	oldExit := exitProcess
	exitProcess = func(code int) { exitCode = code }
	t.Cleanup(func() { exitProcess = oldExit })

	if err := cmd.RunE(cmd, []string{strconv.FormatInt(originalID, 10)}); err != nil {
		t.Fatalf("rerun command: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	latest, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.ID == originalID || latest.Mode != store.ModeRerun || latest.Command != "true" {
		t.Fatalf("latest rerun mismatch: %#v", latest)
	}
}

func TestRerunCmdWithEditFlag(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	originalID := createFinishedRun(t, st, store.StatusSuccess, "true", `["true"]`, t.TempDir())

	t.Setenv("EDITOR", fakeEditor(t, `["echo","edited"]`))

	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg, quiet: true}
	cmd := a.rerunCmd()

	var exitCode int
	oldExit := exitProcess
	exitProcess = func(code int) { exitCode = code }
	t.Cleanup(func() { exitProcess = oldExit })

	if err := cmd.Flags().Set("edit", "true"); err != nil {
		t.Fatalf("set edit flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{strconv.FormatInt(originalID, 10)}); err != nil {
		t.Fatalf("rerun --edit: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	latest, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Command != "echo edited" {
		t.Fatalf("command = %q, want 'echo edited'", latest.Command)
	}
}

func TestEditArgvEditorNotSet(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	_, err := editArgv([]string{"go", "test", "./..."})
	if err == nil {
		t.Fatal("expected error when EDITOR not set")
	}
	if !strings.Contains(err.Error(), "$EDITOR") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEditArgvUsesVISUAL(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	// VISUAL not set either.
	_, err := editArgv([]string{"ls"})
	if err == nil || !strings.Contains(err.Error(), "$EDITOR") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEditArgvEditorExitsNonZero(t *testing.T) {
	// Use a command that exits non-zero.
	t.Setenv("EDITOR", "false")

	_, err := editArgv([]string{"go", "build"})
	if err == nil {
		t.Fatal("expected error when editor exits non-zero")
	}
	if !strings.Contains(err.Error(), "editor exited") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEditArgvUnchanged(t *testing.T) {
	// `true` exits 0 without modifying the temp file → "command unchanged" error.
	t.Setenv("EDITOR", "true")

	_, err := editArgv([]string{"go", "test", "./..."})
	if err == nil {
		t.Fatal("expected error when file is unchanged")
	}
	if !strings.Contains(err.Error(), "unchanged") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEditArgvEmptyArray(t *testing.T) {
	t.Setenv("EDITOR", fakeEditor(t, `[]`))

	_, err := editArgv([]string{"go", "test"})
	if err == nil {
		t.Fatal("expected error for empty argv")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEditArgvValidEdit(t *testing.T) {
	t.Setenv("EDITOR", fakeEditor(t, `["echo","hi"]`))

	got, err := editArgv([]string{"go", "test"})
	if err != nil {
		t.Fatalf("editArgv: %v", err)
	}
	if len(got) != 2 || got[0] != "echo" || got[1] != "hi" {
		t.Fatalf("editArgv = %v", got)
	}
}

// fakeEditor writes an editor that replaces the file it is given with
// content: a shell script, or a .cmd file on Windows.
func fakeEditor(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path, script := filepath.Join(dir, "editor.sh"), "#!/bin/sh\nprintf '%s' 'CONTENT' > \"$1\"\n"
	if runtime.GOOS == "windows" {
		path, script = filepath.Join(dir, "editor.cmd"), "@echo off\r\n>\"%~1\" echo CONTENT\r\n"
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(script, "CONTENT", content)), 0o755); err != nil {
		t.Fatalf("write editor: %v", err)
	}
	return path
}
