package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestBrowseSessionsSource(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_SHELL_STATE", "")
	a, dir := autoSessionApp(t)
	id := newSession(t, a, "", true)
	if _, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "make test", ArgvJSON: `["make test"]`,
		CWD: dir, StartedAt: time.Now(), SessionID: &id,
	}); err != nil {
		t.Fatal(err)
	}
	src := a.browseSessions(a.browseCmd())

	sessions, err := src.List()
	if err != nil || len(sessions) != 1 || sessions[0].ID != id || sessions[0].Runs != 1 {
		t.Fatalf("List = %+v, %v", sessions, err)
	}
	if err := src.Label(id, "BUG-1234: fix login"); err != nil {
		t.Fatal(err)
	}
	if err := src.Keep(id, true); err != nil {
		t.Fatal(err)
	}
	sess, _ := a.st.GetSession(id)
	if sess.Label != "BUG-1234: fix login" || !sess.Kept {
		t.Fatalf("session = %+v", sess)
	}
	if timeline := src.Timeline(id, 80); !strings.Contains(timeline, "session 1 · BUG-1234: fix login") || !strings.Contains(timeline, "make test") {
		t.Fatalf("timeline:\n%s", timeline)
	}
	if timeline := src.Timeline(99, 80); timeline == "" {
		t.Fatal("a missing session should say so")
	}
}
