package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func TestDetachStopsRecordingAndAttachResumes(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_HOOKS_VERSION", "2")
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "BUG-1234: fix login", false)
	if _, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "make", ArgvJSON: `["make"]`,
		CWD: dir, StartedAt: time.Now(), SessionID: &id,
	}); err != nil {
		t.Fatal(err)
	}
	statePath, detachRun := recordedShell(t, a, dir, id, "c detach")

	out, errOut, err := runEmpty(t, a.detachCmd(), nil, nil)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if !strings.Contains(out, `stopped recording this shell; ended session 1 "BUG-1234: fix login"`) || !strings.Contains(errOut, "carrier attach 1") {
		t.Fatalf("output = %q %q", out, errOut)
	}
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if !state.Detached || state.SessionID != 0 {
		t.Fatalf("state = %+v, want detached", state)
	}
	if info, _ := os.ReadFile(carriershell.SessionInfoPath(statePath)); !strings.HasPrefix(string(info), "0\n\n") {
		t.Fatalf("session file = %q, want session 0 for the hooks", info)
	}
	if _, err := a.st.GetRun(detachRun); err == nil {
		t.Fatal("the detach command's own run should be dropped")
	}
	if sess, _ := a.st.GetSession(id); sess.EndedAt == nil {
		t.Fatal("the session should be ended")
	}
	if currentSessionID() != 0 {
		t.Fatalf("currentSessionID = %d in a detached shell", currentSessionID())
	}

	// While detached, the hooks record nothing; carrier ignores them too.
	if _, _, err := runEmpty(t, a.internalBeginCmd(), nil, map[string]string{"state": statePath, "cmd": "ls", "cwd": dir}); err != nil {
		t.Fatal(err)
	}
	if runs, _ := a.st.AllRuns(); len(runs) != 1 {
		t.Fatalf("a detached shell recorded a run: %+v", runs)
	}
	if out := statusOutput(t, a, nil); !strings.Contains(out, `Recording  no; "carrier detach" stopped recording this shell`) {
		t.Fatalf("status:\n%s", out)
	}
	if out, _, _ := runEmpty(t, a.detachCmd(), nil, nil); !strings.Contains(out, "this shell is not recording") {
		t.Fatalf("detach twice: %q", out)
	}

	// attach records into a session again, in the same shell.
	if out, _, err := runEmpty(t, a.attachCmd(), []string{"1234"}, nil); err != nil || !strings.Contains(out, "switched to session 1") {
		t.Fatalf("attach: %q %v", out, err)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.Detached || state.SessionID != id {
		t.Fatalf("state after attach = %+v", state)
	}
	if sess, _ := a.st.GetSession(id); sess.EndedAt != nil {
		t.Fatal("attach should reopen the session")
	}
}

func TestDetachRemovesEmptyAutomaticSession(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_HOOKS_VERSION", "2")
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "", true)
	recordedShell(t, a, dir, id, "carrier detach")
	out, _, err := runEmpty(t, a.detachCmd(), nil, nil)
	if err != nil || !strings.Contains(out, "had nothing in it and was removed") {
		t.Fatalf("detach: %q %v", out, err)
	}
	if _, err := a.st.GetSession(id); err == nil {
		t.Fatal("the empty automatic session should be deleted")
	}
}

// TestDetachKeepsTheLineRunning checks that a line such as "make; carrier
// detach" is still finished by the hooks after it detaches the shell.
func TestDetachKeepsTheLineRunning(t *testing.T) {
	t.Setenv("CARRIER_HOOKS_VERSION", "2")
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "deploy", false)
	statePath, lineRun := recordedShell(t, a, dir, id, "make; carrier detach")
	if _, _, err := runEmpty(t, a.detachCmd(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEmpty(t, a.internalEndCmd(), nil, map[string]string{"state": statePath, "exit": "2"}); err != nil {
		t.Fatal(err)
	}
	r, err := a.st.GetRun(lineRun)
	if err != nil || r.Status != store.StatusFailed {
		t.Fatalf("the line's run should be finished: %+v %v", r, err)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); !state.Detached || state.CurrentID != 0 {
		t.Fatalf("state = %+v, want detached with no run in flight", state)
	}
}

func TestDetachRefusesWhereItCannot(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Setenv("CARRIER_SHELL_STATE", "")
	if _, _, err := runEmpty(t, a.detachCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "not a recorded shell") {
		t.Fatalf("outside a recorded shell: %v", err)
	}
	// Hooks from before carrier detach would keep showing the session.
	t.Setenv("CARRIER_HOOKS_VERSION", "")
	id := newSession(t, a, "deploy", false)
	statePath, _ := recordedShell(t, a, dir, id, "carrier detach")
	if _, _, err := runEmpty(t, a.detachCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "older carrier") {
		t.Fatalf("with old hooks: %v", err)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.Detached {
		t.Fatal("old hooks: the shell should keep recording")
	}
}

func TestFinalSessionOfDetachedShell(t *testing.T) {
	if got := finalSession(5, carriershell.State{Detached: true}); got != 0 {
		t.Fatalf("finalSession = %d, want 0: detach already ended the session", got)
	}
	if got := finalSession(5, carriershell.State{SessionID: 9}); got != 9 {
		t.Fatalf("finalSession = %d, want the session switched to", got)
	}
}
