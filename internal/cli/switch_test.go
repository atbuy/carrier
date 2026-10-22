package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// recordedShell makes the test run as if inside a recorded shell in session
// sessionID, with the hooks' in-flight run for line. It returns the state
// file path and the run ID.
func recordedShell(t *testing.T, a *app, dir string, sessionID int64, line string) (string, int64) {
	t.Helper()
	runID, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: line,
		ArgvJSON: `["` + line + `"]`, CWD: dir, StartedAt: time.Now(), SessionID: &sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state.json")
	if err := (&carriershell.StateFile{Path: statePath}).Write(carriershell.State{CurrentID: runID, SessionID: sessionID}); err != nil {
		t.Fatal(err)
	}
	if err := carriershell.WriteSessionInfo(statePath, sessionID, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CARRIER_SHELL_STATE", statePath)
	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(sessionID))
	return statePath, runID
}

func newSession(t *testing.T, a *app, label string, auto bool) int64 {
	t.Helper()
	id, err := a.st.CreateSession(store.CreateSession{Label: label, StartedAt: time.Now(), Auto: auto})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAttachSwitchesInsideRecordedShell(t *testing.T) {
	a, dir := autoSessionApp(t)
	auto := newSession(t, a, "", true)
	target := newSession(t, a, "BUG-1234: fix login", false)
	if err := a.st.EndSession(target, time.Now()); err != nil {
		t.Fatal(err)
	}
	statePath, attachRun := recordedShell(t, a, dir, auto, "c a 1234")

	out, _, err := runEmpty(t, a.attachCmd(), []string{"1234"}, nil)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if !strings.Contains(out, `switched to session 2 "BUG-1234: fix login"`) {
		t.Fatalf("output = %q", out)
	}
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if state.SessionID != target || state.CurrentID != 0 {
		t.Fatalf("state after switch = %+v, want session %d and no run in flight", state, target)
	}
	info, _ := os.ReadFile(carriershell.SessionInfoPath(statePath))
	if string(info) != "2\nBUG-1234: fix login\n" {
		t.Fatalf("session info = %q", info)
	}
	if _, err := a.st.GetRun(attachRun); err == nil {
		t.Fatal("the attach command's own run should be dropped")
	}
	if _, err := a.st.GetSession(auto); err == nil {
		t.Fatal("the empty automatic session left behind should be deleted")
	}
	if sess, _ := a.st.GetSession(target); sess.EndedAt != nil {
		t.Fatal("the session switched to should be active again")
	}
	if currentSessionID() != target {
		t.Fatalf("currentSessionID = %d, want %d before the prompt updates the environment", currentSessionID(), target)
	}
}

func TestAttachSwitchKeepsSessionWithRuns(t *testing.T) {
	a, dir := autoSessionApp(t)
	old := newSession(t, a, "", true)
	target := newSession(t, a, "deploy", false)
	if _, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "make", ArgvJSON: `["make"]`,
		CWD: dir, StartedAt: time.Now(), SessionID: &old,
	}); err != nil {
		t.Fatal(err)
	}
	recordedShell(t, a, dir, old, "carrier attach deploy")

	if _, _, err := runEmpty(t, a.attachCmd(), []string{"deploy"}, nil); err != nil {
		t.Fatalf("attach: %v", err)
	}
	sess, err := a.st.GetSession(old)
	if err != nil {
		t.Fatalf("a session with runs should be kept: %v", err)
	}
	if sess.EndedAt == nil {
		t.Fatal("the session left behind should be ended")
	}
	if currentSessionID() != target {
		t.Fatalf("currentSessionID = %d, want %d", currentSessionID(), target)
	}
}

func TestAttachSameSession(t *testing.T) {
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "deploy", false)
	recordedShell(t, a, dir, id, "c a deploy")
	out, _, err := runEmpty(t, a.attachCmd(), []string{"deploy"}, nil)
	if err != nil || !strings.Contains(out, "already in session") {
		t.Fatalf("attach to the current session: out=%q err=%v", out, err)
	}
}

func TestAttachNewShellInsideRecordedShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	current := newSession(t, a, "", false)
	target := newSession(t, a, "deploy", false)
	statePath, _ := recordedShell(t, a, dir, current, "c a --new-shell deploy")
	marker := filepath.Join(dir, "child-session")
	program := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf '%s' \"$CARRIER_SESSION_ID\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.cfg.Shell.Program = program

	if _, _, err := runEmpty(t, a.attachCmd(), []string{"deploy"}, map[string]string{"new-shell": "true"}); err != nil {
		t.Fatalf("attach --new-shell: %v", err)
	}
	if got, _ := os.ReadFile(marker); string(got) != fmt.Sprint(target) {
		t.Fatalf("new shell ran in session %q, want %d", got, target)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != current {
		t.Fatalf("the outer shell should stay in session %d, got %+v", current, state)
	}
}

func TestShellEndsTheSessionItSwitchedTo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	target := newSession(t, a, "deploy", false)
	program := filepath.Join(dir, "fake-shell")
	// Stands in for a shell where "carrier attach deploy" switched sessions.
	script := fmt.Sprintf("#!/bin/sh\nprintf '{\"session_id\":%d}' > \"$CARRIER_SHELL_STATE\"\n", target)
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := a.shellCmd()
	_ = cmd.Flags().Set("auto", "true")
	_ = cmd.Flags().Set("program", program)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("shell --auto: %v", err)
	}
	sess, err := a.st.GetSession(target)
	if err != nil || sess.EndedAt == nil {
		t.Fatalf("the session the shell ended in should be ended and kept: %+v, %v", sess, err)
	}
}

func TestShellAutoJoinsPinnedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	target := newSession(t, a, "BUG-1234: fix login", false)
	repo := filepath.Join(dir, "repo")
	sub := filepath.Join(repo, "src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, pinFileName), []byte("# the ticket I am on\nBUG-1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "child-session")
	program := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf '%s' \"$CARRIER_SESSION_ID\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	cmd := a.shellCmd()
	_ = cmd.Flags().Set("auto", "true")
	_ = cmd.Flags().Set("program", program)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("shell --auto: %v", err)
	}
	if got, _ := os.ReadFile(marker); string(got) != fmt.Sprint(target) {
		t.Fatalf("automatic session ran in session %q, want the pinned session %d", got, target)
	}
	if sessions, _ := a.st.ListSessions(10); len(sessions) != 1 {
		t.Fatalf("no new session should be created, got %+v", sessions)
	}
}

func TestShellAutoIgnoresMissingPin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	if err := os.WriteFile(filepath.Join(dir, pinFileName), []byte("OPS-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "child-session")
	program := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf '%s' \"$CARRIER_SESSION_ID\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cmd := a.shellCmd()
	_ = cmd.Flags().Set("auto", "true")
	_ = cmd.Flags().Set("program", program)
	_, errOut, err := runEmpty(t, cmd, nil, nil)
	if err != nil {
		t.Fatalf("shell --auto: %v", err)
	}
	if !strings.Contains(errOut, "ignoring") || !strings.Contains(errOut, `no session with label "OPS-1"`) {
		t.Fatalf("stderr = %q", errOut)
	}
	if got, _ := os.ReadFile(marker); string(got) != "1" {
		t.Fatalf("a new automatic session should start, got session %q", got)
	}
}

func TestFindPin(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if path, _, _ := findPin(sub); path != "" {
		t.Fatalf("no pin yet, got %q", path)
	}
	pin := filepath.Join(dir, "a", pinFileName)
	if err := os.WriteFile(pin, []byte("\n  # comment\n  BUG-1234: fix login  \nignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ref, err := findPin(sub)
	if path != pin || ref != "BUG-1234: fix login" || err != nil {
		t.Fatalf("findPin = %q, %q, %v", path, ref, err)
	}
	if err := os.WriteFile(pin, []byte("# nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := findPin(sub); err == nil {
		t.Fatal("a pin file with no session should be an error")
	}
}

func TestSessionPinAndUnpin(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	t.Setenv("CARRIER_SHELL_STATE", "")
	t.Setenv("CARRIER_SESSION_ID", "")
	unique := newSession(t, a, "BUG-1234: fix login", false)
	shared := newSession(t, a, "debug", false)
	newSession(t, a, "debug", false)
	auto := newSession(t, a, "", true)

	for _, tc := range []struct {
		ref  string
		want string
	}{
		{"1234", "BUG-1234: fix login\n"},
		{fmt.Sprint(shared), fmt.Sprintf("%d\n", shared)},
		{fmt.Sprint(auto), fmt.Sprintf("%d\n", auto)},
	} {
		if _, _, err := runEmpty(t, a.sessionPinCmd(), []string{tc.ref}, nil); err != nil {
			t.Fatalf("pin %s: %v", tc.ref, err)
		}
		if got, _ := os.ReadFile(pinFileName); string(got) != tc.want {
			t.Fatalf("pin %s wrote %q, want %q", tc.ref, got, tc.want)
		}
	}
	if sess, _ := a.st.GetSession(auto); !sess.Kept {
		t.Fatal("a pinned automatic session should be kept")
	}

	if _, _, err := runEmpty(t, a.sessionPinCmd(), nil, nil); err == nil {
		t.Fatal("pin with no session outside a recorded shell should fail")
	}
	recordedShell(t, a, dir, unique, "c session pin")
	if _, _, err := runEmpty(t, a.sessionPinCmd(), nil, nil); err != nil {
		t.Fatalf("pin the current session: %v", err)
	}
	if got, _ := os.ReadFile(pinFileName); string(got) != "BUG-1234: fix login\n" {
		t.Fatalf("pin wrote %q", got)
	}

	if _, _, err := runEmpty(t, a.sessionUnpinCmd(), nil, nil); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if _, err := os.Stat(pinFileName); !os.IsNotExist(err) {
		t.Fatal("unpin should remove the file")
	}
	if _, _, err := runEmpty(t, a.sessionUnpinCmd(), nil, nil); err == nil {
		t.Fatal("unpin with no file should fail")
	}
}

func TestSessionReload(t *testing.T) {
	a, dir := autoSessionApp(t)
	auto := newSession(t, a, "", true)
	target := newSession(t, a, "BUG-1234: fix login", false)
	other := filepath.Join(dir, "other-repo")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	t.Setenv("CARRIER_SHELL_STATE", "")
	if _, _, err := runEmpty(t, a.sessionReloadCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "not in a recorded shell") {
		t.Fatalf("reload outside a recorded shell: %v", err)
	}

	statePath, _ := recordedShell(t, a, dir, auto, "c session reload")
	if _, _, err := runEmpty(t, a.sessionReloadCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "no .carrier-session") {
		t.Fatalf("reload with no pin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pinFileName), []byte("BUG-1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runEmpty(t, a.sessionReloadCmd(), nil, nil)
	if err != nil || !strings.Contains(out, "switched to session") {
		t.Fatalf("reload: out=%q err=%v", out, err)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != target {
		t.Fatalf("state after reload = %+v, want session %d", state, target)
	}
	if _, err := a.st.GetSession(auto); err == nil {
		t.Fatal("the empty automatic session should be deleted after reload")
	}
}

func TestSessionLabelUpdatesPrompt(t *testing.T) {
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "", false)
	statePath, _ := recordedShell(t, a, dir, id, "c session label deploy")
	if _, _, err := runEmpty(t, a.sessionLabelCmd(), []string{"deploy", "fix"}, nil); err != nil {
		t.Fatalf("session label: %v", err)
	}
	if info, _ := os.ReadFile(carriershell.SessionInfoPath(statePath)); string(info) != fmt.Sprintf("%d\ndeploy fix\n", id) {
		t.Fatalf("session info = %q", info)
	}
}

func TestIsSessionCommand(t *testing.T) {
	for line, want := range map[string]bool{
		"carrier attach 5":             true,
		"c a BUG-1234":                 true,
		"c -q a 5":                     true,
		"carrier session reload":       true,
		"c se reload":                  true,
		"carrier session list":         false,
		"c session end -m done":        true,
		"make && c a 5":                false,
		"carrier note attach the logs": false,
		"":                             false,
	} {
		if got := isSessionCommand(line); got != want {
			t.Errorf("isSessionCommand(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestSwitchInShellWithOldHooks(t *testing.T) {
	a, dir := autoSessionApp(t)
	current := newSession(t, a, "", false)
	newSession(t, a, "deploy", false)
	statePath, _ := recordedShell(t, a, dir, current, "c a deploy")
	// Shells started before 1.1.0 have no session file for the hooks to read.
	if err := os.Remove(carriershell.SessionInfoPath(statePath)); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runEmpty(t, a.attachCmd(), []string{"deploy"}, nil)
	if err != nil || !strings.Contains(out, "switched to session") {
		t.Fatalf("attach: out=%q err=%v", out, err)
	}
	if !strings.Contains(errOut, fmt.Sprintf("its prompt still shows session %d", current)) {
		t.Fatalf("stderr = %q", errOut)
	}

	// A current shell gets no hint.
	recordedShell(t, a, dir, current, "c a deploy")
	if _, errOut, _ := runEmpty(t, a.attachCmd(), []string{"deploy"}, nil); strings.Contains(errOut, "older carrier") {
		t.Fatalf("unexpected hint: %q", errOut)
	}
}

// TestInheritedSessionInOtherTerminal covers a tmux pane started from a
// recorded shell: it inherits that shell's variables but runs in another
// terminal, so it must not switch or report the other shell's session.
func TestInheritedSessionInOtherTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test is Unix-only")
	}
	a, dir := autoSessionApp(t)
	outer := newSession(t, a, "", true)
	newSession(t, a, "BUG-20", false)
	statePath, _ := recordedShell(t, a, dir, outer, "c session reload")
	if err := os.WriteFile(filepath.Join(dir, pinFileName), []byte("BUG-20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	ptmx, paneTTY, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer func() { _ = ptmx.Close(); _ = paneTTY.Close() }()
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = paneTTY
	t.Setenv("CARRIER_TTY", "/dev/pts/the-recorded-shell")

	if _, _, err := runEmpty(t, a.sessionReloadCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "not in a recorded shell") {
		t.Fatalf("reload in another terminal: %v", err)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != outer {
		t.Fatalf("the recorded shell's session must not change, state = %+v", state)
	}
	if id := currentSessionID(); id != 0 {
		t.Fatalf("currentSessionID in another terminal = %d, want 0", id)
	}
}
