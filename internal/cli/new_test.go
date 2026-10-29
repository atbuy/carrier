package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func TestNewSwitchesInsideRecordedShell(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	current := newSession(t, a, "BUG-1234: fix login", false)
	if _, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "make", ArgvJSON: `["make"]`,
		CWD: dir, StartedAt: time.Now(), SessionID: &current,
	}); err != nil {
		t.Fatal(err)
	}
	statePath, newRun := recordedShell(t, a, dir, current, "c new BUG-5678: fix signup")

	out, errOut, err := runEmpty(t, a.newCmd(), []string{"BUG-5678:", "fix", "signup"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `started session 2 "BUG-5678: fix signup"`) || !strings.Contains(errOut, "carrier attach 1") {
		t.Fatalf("output = %q %q", out, errOut)
	}
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if state.SessionID != 2 || state.Detached {
		t.Fatalf("state = %+v, want session 2", state)
	}
	if info, _ := os.ReadFile(carriershell.SessionInfoPath(statePath)); !strings.HasPrefix(string(info), "2\nBUG-5678: fix signup\n") {
		t.Fatalf("session file = %q", info)
	}
	sess, err := a.st.GetSession(2)
	if err != nil || sess.Auto || sess.EndedAt != nil {
		t.Fatalf("new session = %+v, %v", sess, err)
	}
	if old, _ := a.st.GetSession(current); old.EndedAt == nil {
		t.Fatal("the session left should be ended")
	}
	if _, err := a.st.GetRun(newRun); err == nil {
		t.Fatal(`the "carrier new" run should be dropped`)
	}
}

func TestNewLeavesEmptyAutomaticSession(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	auto := newSession(t, a, "", true)
	recordedShell(t, a, dir, auto, "carrier new")

	out, errOut, err := runEmpty(t, a.newCmd(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// An unlabeled session; the empty automatic one is gone, so there is
	// nothing to go back to.
	if !strings.Contains(out, "started session 2\n") || strings.Contains(errOut, "carrier attach") {
		t.Fatalf("output = %q %q", out, errOut)
	}
	if _, err := a.st.GetSession(auto); err == nil {
		t.Fatal("the empty automatic session should be deleted")
	}
}

func TestNewWarnsAboutTheSameLabel(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	earlier := newSession(t, a, "BUG-5678: fix signup", false)
	current := newSession(t, a, "", false)
	recordedShell(t, a, dir, current, "c n bug-5678: fix signup")

	_, errOut, err := runEmpty(t, a.newCmd(), []string{"bug-5678:", "fix", "signup"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("session %d is also labeled", earlier); !strings.Contains(errOut, want) || !strings.Contains(errOut, fmt.Sprintf("carrier attach %d", earlier)) {
		t.Fatalf("stderr = %q, want %q", errOut, want)
	}
}

func TestNewOutsideRecordedShellStartsOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	t.Setenv("CARRIER_SHELL_STATE", "")
	t.Setenv("CARRIER_SESSION_ID", "")
	marker := filepath.Join(dir, "child-session")
	program := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf '%s' \"$CARRIER_SESSION_ID\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.cfg.Shell.Program = program

	if _, _, err := runEmpty(t, a.newCmd(), []string{"deploy"}, nil); err != nil {
		t.Fatal(err)
	}
	sess, err := a.st.FindSessionByLabel("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(marker); string(got) != fmt.Sprint(sess.ID) {
		t.Fatalf("the shell ran in session %q, want %d", got, sess.ID)
	}
}

func TestIsSessionCommandIncludesNew(t *testing.T) {
	for _, line := range []string{"carrier new", "c new BUG-1: x", "c n"} {
		if !isSessionCommand(line) {
			t.Errorf("%q should not be recorded", line)
		}
	}
}
