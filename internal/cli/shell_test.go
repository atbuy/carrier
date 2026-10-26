package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func TestShellCmdRunsConfiguredProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	program := filepath.Join(dir, "carrier-shell-cli-test")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf 'cli shell\\n'\n"), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}

	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Shell.Program = program

	st, err := store.Open(cfg.Storage.DataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{cfg: cfg, st: st}
	cmd := a.shellCmd()
	if err := cmd.Args(cmd, []string{"label1", "label2"}); err == nil {
		t.Fatal("expected max-one-arg validation error")
	}
	if err := cmd.Args(cmd, []string{"mylabel"}); err != nil {
		t.Fatalf("expected one arg to be valid: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("shell command: %v", err)
	}
}

func TestShellCmdFinishesRunInFlightAtExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	st, err := store.Open(cfg.Storage.DataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// The run the hooks began for "exit 3"; the shell exits before ending it.
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "exit 3",
		ArgvJSON: `["exit 3"]`, CWD: dir, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	program := filepath.Join(dir, "carrier-shell-exit-test")
	script := fmt.Sprintf("#!/bin/sh\nprintf '{\"current_id\":%d}' > \"$CARRIER_SHELL_STATE\"\nexit 3\n", id)
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}
	cfg.Shell.Program = program

	a := &app{cfg: cfg, st: st}
	cmd := a.shellCmd()
	_ = cmd.RunE(cmd, nil)

	r, err := st.GetRun(id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.Status != store.StatusFailed || r.ExitCode == nil || *r.ExitCode != 3 {
		t.Fatalf("in-flight run should be finished as failed with exit 3, got status=%s exit=%v", r.Status, r.ExitCode)
	}
}

func TestFinishInFlightShellRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh to produce exit statuses")
	}
	exitErr := func(script string) error {
		return exec.Command("sh", "-c", script).Run()
	}
	cases := []struct {
		name     string
		runErr   error
		status   string
		exitCode int
	}{
		{"clean exit", nil, store.StatusSuccess, 0},
		{"exit status", exitErr("exit 2"), store.StatusFailed, 2},
		{"killed by signal", exitErr("kill -9 $$"), store.StatusKilled, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openCmdStore(t)
			defer func() { _ = st.Close() }()
			id, err := st.CreateRun(store.CreateRun{
				Status: store.StatusRunning, Mode: store.ModeShell, Command: "exit",
				ArgvJSON: `["exit"]`, CWD: "/tmp", StartedAt: time.Now(),
			})
			if err != nil {
				t.Fatalf("create run: %v", err)
			}
			a := &app{st: st}
			a.finishInFlightShellRun(carriershell.State{CurrentID: id}, tc.runErr)
			r, err := st.GetRun(id)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if r.Status != tc.status || r.ExitCode == nil || *r.ExitCode != tc.exitCode {
				t.Fatalf("got status=%s exit=%v, want %s/%d", r.Status, r.ExitCode, tc.status, tc.exitCode)
			}
		})
	}

	// No run in flight: nothing to do.
	(&app{}).finishInFlightShellRun(carriershell.State{}, nil)
}

func TestShellRunsCutShort(t *testing.T) {
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "", true)
	start := func(command string) int64 {
		runID, err := a.st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: `[]`,
			CWD: dir, StartedAt: time.Now().Add(-time.Minute), SessionID: &id,
		})
		if err != nil {
			t.Fatal(err)
		}
		return runID
	}

	// The terminal closed while the command ran.
	hungUp := start("sleep 100")
	a.finishInFlightShellRun(carriershell.State{CurrentID: hungUp}, carriershell.ErrHungUp)
	if r, _ := a.st.GetRun(hungUp); r.Status != store.StatusKilled || r.ExitCode == nil || *r.ExitCode != 129 {
		t.Fatalf("run cut short by a hangup = %+v", r)
	}

	// The shell was killed outright; the temp cleanup found its state.
	killed := start("make test")
	logPath := filepath.Join(dir, "make.log")
	if err := os.WriteFile(logPath, []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.st.UpdatePaths(killed, "", "", logPath); err != nil {
		t.Fatal(err)
	}
	finished := start("ls")
	if err := a.st.FinishRun(finished, store.StatusSuccess, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	a.finishKilledShells([]carriershell.State{{CurrentID: killed, SessionID: id}, {CurrentID: finished, SessionID: id}})
	r, _ := a.st.GetRun(killed)
	if r.Status != store.StatusKilled || r.DurationMS == nil || *r.DurationMS < 50_000 {
		t.Fatalf("run of a killed shell = %+v; want killed, lasting until its output was last written", r)
	}
	if r, _ := a.st.GetRun(finished); r.Status != store.StatusSuccess {
		t.Fatalf("a finished run should be left alone: %+v", r)
	}
	if sess, _ := a.st.GetSession(id); sess.EndedAt == nil {
		t.Fatal("the killed shell's session should be ended")
	}
}
