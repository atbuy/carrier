package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func autoSessionApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	st, err := store.Open(cfg.Storage.DataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &app{cfg: cfg, st: st}, dir
}

func TestShellAutoDeletesEmptySessionAndIgnoresExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	program := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write program: %v", err)
	}

	cmd := a.shellCmd()
	_ = cmd.Flags().Set("auto", "true")
	_ = cmd.Flags().Set("program", program)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("shell --auto should succeed when the shell exits, got %v", err)
	}
	if sessions, _ := a.st.ListSessions(10); len(sessions) != 0 {
		t.Fatalf("empty automatic session should be deleted, got %+v", sessions)
	}

	manual := a.shellCmd()
	_ = manual.Flags().Set("program", program)
	if err := manual.RunE(manual, nil); err == nil {
		t.Fatal("plain carrier shell should still report the shell's exit status")
	}
	sessions, _ := a.st.ListSessions(10)
	if len(sessions) != 1 || sessions[0].Auto {
		t.Fatalf("manual session should be kept and not auto, got %+v", sessions)
	}
}

func TestShellAutoReportsStartupFailure(t *testing.T) {
	a, _ := autoSessionApp(t)
	a.cfg.Shell.Program = "/nonexistent/shell"
	cmd := a.shellCmd()
	_ = cmd.Flags().Set("auto", "true")
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "automatic session not started:") {
		t.Fatalf("startup failure should be reported, got %v", err)
	}
	if autoStartError(false, errors.New("x")).Error() != "x" || autoStartError(true, nil) != nil {
		t.Fatal("autoStartError should only wrap automatic session errors")
	}
}

func TestDropExitRun(t *testing.T) {
	a, dir := autoSessionApp(t)
	sessID, _ := a.st.CreateSession(store.CreateSession{StartedAt: time.Now(), Auto: true})
	mkRun := func(command string) (int64, string) {
		id, err := a.st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: `["x"]`,
			CWD: "/tmp", StartedAt: time.Now(), SessionID: &sessID,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		log := filepath.Join(dir, fmt.Sprintf("%d.log", id))
		_ = os.WriteFile(log, []byte("x"), 0o600)
		_ = a.st.UpdatePaths(id, "", "", log)
		return id, log
	}

	exitID, exitLog := mkRun("exit")
	if !a.dropExitRun(carriershell.State{CurrentID: exitID}) {
		t.Fatal("exit run should be dropped")
	}
	if _, err := a.st.GetRun(exitID); err == nil {
		t.Fatal("exit run should be deleted")
	}
	if _, err := os.Stat(exitLog); !os.IsNotExist(err) {
		t.Fatal("exit run log should be removed")
	}

	makeID, _ := mkRun("make test")
	if a.dropExitRun(carriershell.State{CurrentID: makeID}) {
		t.Fatal("other commands must not be dropped")
	}
	if a.dropExitRun(carriershell.State{}) {
		t.Fatal("no run in flight: nothing to drop")
	}
	for cmd, want := range map[string]bool{"exit": true, "exit 3": true, "logout": true, "exit && x": false, "exiting": false, "": false} {
		if got := isExitCommand(cmd); got != want {
			t.Errorf("isExitCommand(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// oldAutoSession records an automatic session that ended 60 days ago, with
// one run whose log exists on disk.
func oldAutoSession(t *testing.T, a *app, dir, label string) (int64, string) {
	t.Helper()
	old := time.Now().Add(-60 * 24 * time.Hour)
	id, err := a.st.CreateSession(store.CreateSession{Label: label, StartedAt: old, Auto: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = a.st.EndSession(id, old.Add(time.Hour))
	runID, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "ls", ArgvJSON: `["ls"]`,
		CWD: "/tmp", StartedAt: old, SessionID: &id,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	log := filepath.Join(dir, fmt.Sprintf("session-%d.log", id))
	_ = os.WriteFile(log, []byte("output"), 0o600)
	_ = a.st.UpdatePaths(runID, "", "", log)
	return id, log
}

func TestExpireAutoSessions(t *testing.T) {
	a, dir := autoSessionApp(t)
	expired, log := oldAutoSession(t, a, dir, "")
	kept, _ := oldAutoSession(t, a, dir, "BUG-7 keep me")

	sessions, runs, err := a.expireAutoSessions(time.Now(), true)
	if err != nil || len(sessions) != 1 || sessions[0].ID != expired || len(runs) != 1 {
		t.Fatalf("dry run = %+v, %d runs, %v", sessions, len(runs), err)
	}
	if _, err := a.st.GetSession(expired); err != nil {
		t.Fatal("dry run must not delete")
	}

	if _, _, err := a.expireAutoSessions(time.Now(), false); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, err := a.st.GetSession(expired); err == nil {
		t.Fatal("expired session should be deleted")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("expired session logs should be removed")
	}
	if _, err := a.st.GetSession(kept); err != nil {
		t.Fatal("labeled session must be kept")
	}

	a.cfg.Shell.AutoSessionRetention = "0"
	again, _ := oldAutoSession(t, a, dir, "")
	if sessions, _, _ := a.expireAutoSessions(time.Now(), false); len(sessions) != 0 {
		t.Fatal("retention 0 keeps everything")
	}
	if _, err := a.st.GetSession(again); err != nil {
		t.Fatal("retention 0 must not delete")
	}
}

func TestExpireAutoSessionsIfDueIsThrottled(t *testing.T) {
	a, dir := autoSessionApp(t)
	first, _ := oldAutoSession(t, a, dir, "")
	a.expireAutoSessionsIfDue(a.shellCmd())
	if _, err := a.st.GetSession(first); err == nil {
		t.Fatal("first check should expire the old session")
	}
	second, _ := oldAutoSession(t, a, dir, "")
	a.expireAutoSessionsIfDue(a.shellCmd())
	if _, err := a.st.GetSession(second); err != nil {
		t.Fatal("cleanup runs at most once a day")
	}
}

func TestCleanSessions(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	id, _ := oldAutoSession(t, a, dir, "")

	if _, _, err := runEmpty(t, a.cleanCmd(), nil, map[string]string{"sessions": "true"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("clean --sessions without --yes: %v", err)
	}
	out, _, err := runEmpty(t, a.cleanCmd(), nil, map[string]string{"sessions": "true", "dry-run": "true"})
	if err != nil || !strings.Contains(out, "would delete 1 expired automatic sessions (1 runs)") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	out, _, err = runEmpty(t, a.cleanCmd(), nil, map[string]string{"sessions": "true", "yes": "true"})
	if err != nil || !strings.Contains(out, "deleted 1 expired automatic sessions") {
		t.Fatalf("clean --sessions --yes: %v\n%s", err, out)
	}
	if _, err := a.st.GetSession(id); err == nil {
		t.Fatal("session should be deleted")
	}
}

func TestSessionKeepAndList(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, _ := autoSessionApp(t)
	auto, _ := a.st.CreateSession(store.CreateSession{Label: "api-debug", StartedAt: time.Now(), Auto: true})
	other, _ := a.st.CreateSession(store.CreateSession{StartedAt: time.Now(), Auto: true})

	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(other))
	if out, _, err := runEmpty(t, a.sessionKeepCmd(true), nil, nil); err != nil || !strings.Contains(out, fmt.Sprintf("session %d kept", other)) {
		t.Fatalf("keep current: %v %q", err, out)
	}
	if out, _, err := runEmpty(t, a.sessionKeepCmd(true), []string{"api-debug"}, nil); err != nil || !strings.Contains(out, fmt.Sprintf("session %d kept", auto)) {
		t.Fatalf("keep by label: %v %q", err, out)
	}
	out, _, _ := runEmpty(t, a.sessionListCmd(), nil, nil)
	if strings.Count(out, "auto kept") != 2 {
		t.Fatalf("list should mark auto and kept sessions:\n%s", out)
	}
	if out, _, err := runEmpty(t, a.sessionKeepCmd(false), []string{fmt.Sprint(auto)}, nil); err != nil || !strings.Contains(out, "no longer kept") {
		t.Fatalf("unkeep: %v %q", err, out)
	}
	if s, _ := a.st.GetSession(auto); s.Kept {
		t.Fatal("unkeep should clear kept")
	}

	t.Setenv("CARRIER_SESSION_ID", "")
	if _, _, err := runEmpty(t, a.sessionKeepCmd(true), nil, nil); err == nil || !strings.Contains(err.Error(), "CARRIER_SESSION_ID") {
		t.Fatalf("keep without session: %v", err)
	}
	if _, _, err := runEmpty(t, a.sessionKeepCmd(true), []string{"9999"}, nil); err == nil {
		t.Fatal("keep missing session should fail")
	}
}

func TestSessionLabelHintForAutoSessions(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, _ := autoSessionApp(t)
	auto, _ := a.st.CreateSession(store.CreateSession{StartedAt: time.Now(), Auto: true})
	_, errOut, err := runEmpty(t, a.sessionLabelCmd(), []string{fmt.Sprint(auto), "api", "debug"}, nil)
	if err != nil || !strings.Contains(errOut, "labeled automatic sessions are kept") {
		t.Fatalf("label auto session: %v %q", err, errOut)
	}
	manual, _ := a.st.CreateSession(store.CreateSession{StartedAt: time.Now()})
	if _, errOut, _ := runEmpty(t, a.sessionLabelCmd(), []string{fmt.Sprint(manual), "x"}, nil); errOut != "" {
		t.Fatalf("manual sessions need no hint, got %q", errOut)
	}
}

func TestInitCmd(t *testing.T) {
	a := &app{}
	out, _, err := runEmpty(t, a.initCmd(), []string{"zsh"}, nil)
	if err != nil || !strings.Contains(out, "shell --auto --program zsh && exit") {
		t.Fatalf("init zsh: %v\n%s", err, out)
	}
	t.Setenv("SHELL", "/usr/bin/fish")
	if out, _, err := runEmpty(t, a.initCmd(), nil, nil); err != nil || !strings.Contains(out, "--program fish; and exit") {
		t.Fatalf("init from $SHELL: %v\n%s", err, out)
	}
	t.Setenv("SHELL", "/bin/sh")
	if _, _, err := runEmpty(t, a.initCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "could not tell your shell") {
		t.Fatalf("unknown $SHELL: %v", err)
	}
	if _, _, err := runEmpty(t, a.initCmd(), []string{"tcsh"}, nil); err == nil {
		t.Fatal("tcsh should be rejected")
	}
}

func TestAutoSessionStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	if got := autoSessionStatus("/bin/bash", "30d"); got != `off; add eval "$(carrier init bash)" to ~/.bashrc` {
		t.Fatalf("off status = %q", got)
	}
	if got := autoSessionStatus("/bin/sh", "30d"); got != "off; see carrier init" {
		t.Fatalf("unsupported shell status = %q", got)
	}
	_ = os.WriteFile(filepath.Join(home, ".zshrc"), []byte("eval \"$(carrier init zsh)\"\n"), 0o600)
	if got := autoSessionStatus("/bin/bash", "30d"); got != "on in ~/.zshrc (kept 30d)" {
		t.Fatalf("on status = %q", got)
	}
}
