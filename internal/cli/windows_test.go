package cli

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

// TestRecordedShellsOnWindows checks that the commands that start a recorded
// shell say plainly that Windows has none, and leave no session behind.
func TestRecordedShellsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		if err := carriershell.Supported(); err != nil {
			t.Fatalf("recorded shells should be supported on %s: %v", runtime.GOOS, err)
		}
		t.Skip("recorded shells work here")
	}
	a, _ := autoSessionApp(t)
	t.Setenv("CARRIER_SHELL_STATE", "")
	for name, run := range map[string]func() error{
		"shell": func() error { _, _, err := runEmpty(t, a.shellCmd(), nil, nil); return err },
		"init":  func() error { _, _, err := runEmpty(t, a.initCmd(), []string{"bash"}, nil); return err },
	} {
		if err := run(); !errors.Is(err, carriershell.ErrUnsupported) {
			t.Errorf("%s: err = %v, want the Windows message", name, err)
		}
	}
	id := newSession(t, a, "deploy", false)
	if _, _, err := runEmpty(t, a.attachCmd(), []string{"deploy"}, nil); !errors.Is(err, carriershell.ErrUnsupported) {
		t.Errorf("attach: err = %v", err)
	}
	if _, _, err := runEmpty(t, a.shellCmd(), nil, map[string]string{"auto": "true"}); err == nil || !strings.Contains(err.Error(), "carrier run") {
		t.Errorf("shell --auto: err = %v", err)
	}
	if out, _, _ := runEmpty(t, a.doctorCmd(), nil, nil); !strings.Contains(out, "not supported on Windows") {
		t.Errorf("doctor should say recorded shells do not work here:\n%s", out)
	}
	sessions, _ := a.st.ListSessions(100)
	if len(sessions) != 1 || sessions[0].ID != id {
		t.Fatalf("no session should be created: %+v", sessions)
	}
}
