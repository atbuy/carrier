package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestNotifiesInShell(t *testing.T) {
	ignore := []string{"vim", "less"}
	for command, want := range map[string]bool{
		"go test ./...":            true,
		"make build":               true,
		"vim main.go":              false,
		"/usr/bin/less log.txt":    false,
		"ssh db1":                  false,
		"mosh db1":                 false,
		"carrier browse":           false,
		"c config":                 false,
		"carrier tail 12":          false,
		"carrier run make release": true,
		"c -n rr 42":               true,
		"sleep 30":                 true,
	} {
		if got := notifiesInShell(command, 0, ignore); got != want {
			t.Errorf("notifiesInShell(%q) = %v, want %v", command, got, want)
		}
	}
	if notifiesInShell("go test ./...", 130, ignore) {
		t.Error("a command stopped with Ctrl+C should not notify")
	}
}

// shellRunToNotify records a finished shell run of command that took
// duration in a labeled session, and returns it.
func shellRunToNotify(t *testing.T, a *app, command string, duration time.Duration, exitCode int, notifyRequested bool) store.Run {
	t.Helper()
	id := newSession(t, a, "BUG-1234: fix login", false)
	started := time.Now().Add(-duration)
	runID, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: `[]`,
		CWD: "/src/api", StartedAt: started, SessionID: &id, NotifyRequested: notifyRequested,
	})
	if err != nil {
		t.Fatal(err)
	}
	status := store.StatusSuccess
	if exitCode != 0 {
		status = store.StatusFailed
	}
	if err := a.st.FinishRun(runID, status, exitCode, time.Now()); err != nil {
		t.Fatal(err)
	}
	r, err := a.st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	return *r
}

func captureNotifications(t *testing.T) *[]string {
	t.Helper()
	var sent []string
	old := sendNotification
	sendNotification = func(title, body string) error {
		sent = append(sent, title+"\n"+body)
		return nil
	}
	t.Cleanup(func() { sendNotification = old })
	return &sent
}

func TestNotifyShellRun(t *testing.T) {
	a, _ := autoSessionApp(t)
	sent := captureNotifications(t)

	// Off, the default: only shells started with -n notify.
	a.notifyShellRun(shellRunToNotify(t, a, "go test ./...", time.Minute, 1, false), 1)
	if len(*sent) != 0 {
		t.Fatalf("notify.shell is off, sent %q", *sent)
	}
	a.notifyShellRun(shellRunToNotify(t, a, "go test ./...", time.Minute, 1, true), 1)
	if len(*sent) != 1 {
		t.Fatalf("a -n shell should notify, sent %q", *sent)
	}

	a.cfg.Notify.Shell = true
	*sent = nil
	a.notifyShellRun(shellRunToNotify(t, a, "go test ./...", 2*time.Minute, 1, false), 1)
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "carrier: command failed\ngo test ./...") || !strings.Contains((*sent)[0], "/src/api · BUG-1234: fix login") {
		t.Fatalf("a long failed command should notify with where it ran, sent %q", *sent)
	}
	for _, skip := range []struct {
		command  string
		duration time.Duration
		exit     int
	}{
		{"go build", time.Second, 0},    // shorter than notify.min_duration
		{"vim notes.md", time.Hour, 0},  // shell.ignore_commands
		{"make test", time.Minute, 130}, // stopped with Ctrl+C
	} {
		*sent = nil
		a.notifyShellRun(shellRunToNotify(t, a, skip.command, skip.duration, skip.exit, false), skip.exit)
		if len(*sent) != 0 {
			t.Errorf("%q should not notify, sent %q", skip.command, *sent)
		}
	}
	a.cfg.Notify.Success = false
	*sent = nil
	a.notifyShellRun(shellRunToNotify(t, a, "make release", time.Minute, 0, false), 0)
	if len(*sent) != 0 {
		t.Fatalf("notify.success = false should hold back successes, sent %q", *sent)
	}
}
