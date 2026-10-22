package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/creack/pty"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func statusOutput(t *testing.T, a *app, flags map[string]string) string {
	t.Helper()
	out, _, err := runEmpty(t, a.statusCmd(), nil, flags)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	return out
}

func TestStatusNotRecorded(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_SHELL_STATE", "")
	t.Setenv("CARRIER_SESSION_ID", "")
	t.Setenv("TMUX", "")
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	out := statusOutput(t, a, nil)
	if !strings.Contains(out, "Recording  no; this terminal is not a recorded shell") || !strings.Contains(out, "Pin here   none") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestStatusRecordedWithPinElsewhere(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("TMUX", "")
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	current := newSession(t, a, "", true)
	pinned := newSession(t, a, "BUG-20: pinned", false)
	recordedShell(t, a, dir, current, "make test")
	if err := os.WriteFile(pinFileName, []byte("BUG-20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := statusOutput(t, a, nil)
	for _, want := range []string{
		"Recording  session 1",
		"active, started just now · 1 command",
		"automatic; label, note, or keep it",
		"Pin here   " + filepath.Join(dir, pinFileName) + " → session 2 · BUG-20: pinned",
		`"carrier session reload" switches to the pinned one`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	var v statusView
	if err := json.Unmarshal([]byte(statusOutput(t, a, map[string]string{"json": "true"})), &v); err != nil {
		t.Fatal(err)
	}
	if !v.Recorded || v.Session == nil || v.Session.ID != current || v.RunCount != 1 || v.Pin == nil || v.Pin.SessionID != pinned {
		t.Fatalf("status JSON = %+v", v)
	}
}

func TestStatusOldHooksAndMissingPin(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("TMUX", "")
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	id := newSession(t, a, "deploy", false)
	statePath, _ := recordedShell(t, a, dir, id, "make")
	if err := os.Remove(carriershell.SessionInfoPath(statePath)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinFileName, []byte("NOPE-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := statusOutput(t, a, nil)
	if !strings.Contains(out, "Prompt     started by a carrier older than 1.1.0") || !strings.Contains(out, `no session with label "NOPE-1"`) {
		t.Fatalf("status:\n%s", out)
	}
}

func TestStatusInheritedFromOtherTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test is Unix-only")
	}
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("TMUX", "")
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	id := newSession(t, a, "", true)
	recordedShell(t, a, dir, id, "make")
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer func() { _ = ptmx.Close(); _ = tty.Close() }()
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = tty
	t.Setenv("CARRIER_TTY", "/dev/pts/the-recorded-shell")

	out := statusOutput(t, a, nil)
	if !strings.Contains(out, "Recording  no") || !strings.Contains(out, "inherited session 1's variables") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestTmuxEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only")
	}
	bin := t.TempDir()
	envFile := filepath.Join(bin, "env")
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\ncat "+envFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	t.Setenv("TMUX", "")
	if _, _, applies := tmuxEnvironment(); applies {
		t.Fatal("outside tmux the check does not apply")
	}
	t.Setenv("TMUX", "/tmp/tmux-0/default,1,0")
	for env, wantOK := range map[string]bool{
		"HOME=/home/dev\n": true,
		"CARRIER_SESSION_ID=4\nCARRIER_TTY=/dev/pts/3\n": true,
		"CARRIER_SESSION_ID=48\n":                        false,
		"ZDOTDIR=/tmp/carrier-shell-473385145\n":         false,
	} {
		if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
		ok, detail, applies := tmuxEnvironment()
		if !applies || ok != wantOK {
			t.Errorf("tmux environment %q: ok=%v applies=%v (%s)", env, ok, applies, detail)
		}
		if !ok && !strings.Contains(detail, "tmux kill-server") {
			t.Errorf("the warning should say how to fix it: %s", detail)
		}
	}
}

func TestDoctorReportsShellAndPin(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("TMUX", "")
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	id := newSession(t, a, "deploy", false)
	recordedShell(t, a, dir, id, "make")
	if err := os.WriteFile(pinFileName, []byte("NOPE-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := a.runDoctor(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`this shell   ok  recording session 1 "deploy"`, "pin          warn", `no session with label "NOPE-1"`, "temp files   ok"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor missing %q:\n%s", want, out.String())
		}
	}
}
