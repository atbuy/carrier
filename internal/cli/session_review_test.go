package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// reviewSession records a labeled session with a passing run, a failed run
// with error output, a run in another directory, a note, and an outcome.
func reviewSession(t *testing.T, a *app, dir string) int64 {
	t.Helper()
	start := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
	id, err := a.st.CreateSession(store.CreateSession{Label: "BUG-1234: fix login", StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	logPath := func(name, text string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := func(n int, command, cwd, branch, log, status string, exit int) {
		t.Helper()
		at := start.Add(time.Duration(n) * time.Minute)
		rid, err := a.st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: `["x"]`,
			CWD: cwd, GitBranch: branch, StartedAt: at, SessionID: &id,
			TerminalOutputPath: logPath(fmt.Sprintf("%d.log", n), log),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.st.FinishRun(rid, status, exit, at.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	run(1, "make test", "/src/api", "fix-login", "ok\n", store.StatusSuccess, 0)
	run(2, "psql -h db", "/src/api", "fix-login", "connecting\r\nerror: connection refused\r\n", store.StatusFailed, 2)
	run(4, "kubectl get pods", "/src/infra", "main", "\x1b[32mdb-0 Running\x1b[0m\r\n", store.StatusSuccess, 0)
	for _, n := range []store.CreateNote{
		{SessionID: &id, Text: "the database was restarting", CreatedAt: start.Add(3 * time.Minute)},
		{SessionID: &id, Text: "pool size was 0", Kind: store.NoteOutcome, CreatedAt: start.Add(10 * time.Minute)},
	} {
		if _, err := a.st.CreateNote(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.st.EndSession(id, start.Add(80*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSessionShow(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	reviewSession(t, a, dir)

	out, _, err := runEmpty(t, a.sessionShowCmd(), []string{"1234"}, nil)
	if err != nil {
		t.Fatalf("session show: %v", err)
	}
	for _, want := range []string{
		"session 1 · BUG-1234: fix login",
		"(1h 20m) · 3 commands, 1 failed · 2 notes · /src/api on fix-login",
		"Outcome: pool size was 0",
		"✓  make test  2s",
		"✗  psql -h db  exit 2  2s",
		"│ error: connection refused",
		"✎  the database was restarting",
		"       /src/infra on main",
		"✎  outcome: pool size was 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("session show missing %q:\n%s", want, out)
		}
	}
	assertInOrder(t, out, "make test", "psql -h db", "the database was restarting", "kubectl get pods", "outcome: pool size")
	if strings.Contains(out, "connecting") || strings.Contains(out, "db-0 Running") {
		t.Errorf("only failed runs show output by default:\n%s", out)
	}

	out, _, _ = runEmpty(t, a.sessionShowCmd(), []string{"1234"}, map[string]string{"failed": "true"})
	if strings.Contains(out, "make test") || !strings.Contains(out, "psql -h db") || !strings.Contains(out, "the database was restarting") {
		t.Errorf("--failed should keep failed runs and notes only:\n%s", out)
	}

	out, _, _ = runEmpty(t, a.sessionShowCmd(), []string{"1234"}, map[string]string{"output": "true", "lines": "1"})
	if !strings.Contains(out, "│ db-0 Running\n") || strings.Contains(out, "\x1b") {
		t.Errorf("--output should show the last line of every run, without escapes:\n%q", out)
	}
}

func TestSessionShowJSON(t *testing.T) {
	a, dir := autoSessionApp(t)
	reviewSession(t, a, dir)
	out, _, err := runEmpty(t, a.sessionShowCmd(), []string{"1234"}, map[string]string{"json": "true"})
	if err != nil {
		t.Fatal(err)
	}
	var v sessionShowView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if v.Label != "BUG-1234: fix login" || v.RunCount != 3 || v.FailedCount != 1 || v.NoteCount != 2 || v.Outcome == nil || v.Outcome.Kind != "outcome" {
		t.Fatalf("summary = %+v", v)
	}
	if len(v.Timeline) != 5 || v.Timeline[1].Type != "run" || len(v.Timeline[1].Excerpt) != 1 || v.Timeline[2].Type != "note" {
		t.Fatalf("timeline = %+v", v.Timeline)
	}
}

func TestSessionShowDefaultsToCurrentOrPinned(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, dir := autoSessionApp(t)
	reviewSession(t, a, dir)
	t.Chdir(dir)
	t.Setenv("CARRIER_SHELL_STATE", "")
	t.Setenv("CARRIER_SESSION_ID", "")
	if _, _, err := runEmpty(t, a.sessionShowCmd(), nil, nil); err == nil {
		t.Fatal("no session to show should be an error")
	}
	if err := os.WriteFile(pinFileName, []byte("BUG-1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runEmpty(t, a.sessionShowCmd(), nil, nil); err != nil || !strings.Contains(out, "BUG-1234: fix login") {
		t.Fatalf("pinned session: out=%q err=%v", out, err)
	}
	other := newSession(t, a, "other", false)
	recordedShell(t, a, dir, other, "c session show")
	if out, _, err := runEmpty(t, a.sessionShowCmd(), nil, nil); err != nil || !strings.Contains(out, "session 2 · other") {
		t.Fatalf("the shell's session should win over the pin: out=%q err=%v", out, err)
	}
}

func TestSessionShowEmpty(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a, _ := autoSessionApp(t)
	newSession(t, a, "empty", false)
	out, _, err := runEmpty(t, a.sessionShowCmd(), []string{"empty"}, nil)
	if err != nil || !strings.Contains(out, "now, active · 0 commands") || !strings.Contains(out, "nothing recorded") {
		t.Fatalf("empty session: out=%q err=%v", out, err)
	}
}

func TestSessionExportMarkdown(t *testing.T) {
	a, dir := autoSessionApp(t)
	reviewSession(t, a, dir)
	out, _, err := runEmpty(t, a.sessionExportCmd(), []string{"1234"}, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, want := range []string{
		"# BUG-1234: fix login\n",
		"**Outcome:** pool size was 0\n",
		"3 commands, 1 failed · 2 notes · `/src/api` on fix-login",
		"## Timeline",
		"`make test` (2s)",
		"`psql -h db` (failed, exit 2, 2s)\n\n  ```text\n  error: connection refused\n  ```\n",
		"_Note:_ the database was restarting",
		"in `/src/infra` on `main`: `kubectl get pods`",
		"_Outcome:_ pool size was 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\r") {
		t.Errorf("export should be plain text:\n%q", out)
	}
}

func TestSessionExportToFileAndJSON(t *testing.T) {
	a, dir := autoSessionApp(t)
	reviewSession(t, a, dir)
	path := filepath.Join(dir, "bug.md")
	out, errOut, err := runEmpty(t, a.sessionExportCmd(), []string{"1234"}, map[string]string{"output-file": path})
	if err != nil || out != "" || !strings.Contains(errOut, "wrote session 1") {
		t.Fatalf("export -o: out=%q errOut=%q err=%v", out, errOut, err)
	}
	if b, _ := os.ReadFile(path); !strings.HasPrefix(string(b), "# BUG-1234: fix login") {
		t.Fatalf("file = %q", b)
	}

	out, _, err = runEmpty(t, a.sessionExportCmd(), []string{"1234"}, map[string]string{"format": "json"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, dir) || strings.Contains(out, "terminal_output_path") {
		t.Fatalf("JSON export should not contain log paths:\n%s", out)
	}
	if _, _, err := runEmpty(t, a.sessionExportCmd(), []string{"1234"}, map[string]string{"format": "csv"}); err == nil {
		t.Fatal("unknown format should fail")
	}
}

func TestSessionExportShortensHome(t *testing.T) {
	a, dir := autoSessionApp(t)
	home := filepath.Join(dir, "home")
	setHome(t, home)
	id := newSession(t, a, "", false)
	at := time.Now()
	rid, err := a.st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "cat `notes`", ArgvJSON: `["x"]`,
		CWD: filepath.Join(home, "api"), StartedAt: at, SessionID: &id,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.FinishRun(rid, store.StatusSuccess, 0, at); err != nil {
		t.Fatal(err)
	}
	out, _, err := runEmpty(t, a.sessionExportCmd(), []string{fmt.Sprint(id)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, home) || !strings.Contains(out, "`"+filepath.FromSlash("~/api")+"`") || !strings.Contains(out, "`` cat `notes` ``") {
		t.Fatalf("export:\n%s", out)
	}
}

func TestSessionEndOutsideShell(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	t.Setenv("CARRIER_SHELL_STATE", "")
	id := newSession(t, a, "BUG-1234: fix login", false)
	if err := os.WriteFile(pinFileName, []byte("BUG-1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runEmpty(t, a.sessionEndCmd(), []string{"1234"}, map[string]string{"note": "pool size was 0"})
	if err != nil || !strings.Contains(out, `ended session 1 "BUG-1234: fix login"`) {
		t.Fatalf("end: out=%q err=%v", out, err)
	}
	if !strings.Contains(errOut, "still join this session") {
		t.Fatalf("a pin for the ended session should be pointed out, stderr=%q", errOut)
	}
	if sess, _ := a.st.GetSession(id); sess.EndedAt == nil {
		t.Fatal("session should be ended")
	}
	if o, _ := a.st.SessionOutcome(id); o == nil || o.Text != "pool size was 0" {
		t.Fatalf("outcome = %+v", o)
	}

	if _, _, err := runEmpty(t, a.sessionEndCmd(), []string{"1234"}, map[string]string{"unpin": "true"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pinFileName); !os.IsNotExist(err) {
		t.Fatal("--unpin should remove the pin")
	}
}

func TestSessionEndKeepsOtherPins(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	t.Setenv("CARRIER_SHELL_STATE", "")
	newSession(t, a, "BUG-1234", false)
	newSession(t, a, "OPS-7", false)
	if err := os.WriteFile(pinFileName, []byte("OPS-7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEmpty(t, a.sessionEndCmd(), []string{"BUG-1234"}, map[string]string{"unpin": "true"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(pinFileName); string(b) != "OPS-7\n" {
		t.Fatalf("a pin for another session must stay, got %q", b)
	}
}

func TestSessionEndInsideShell(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Chdir(dir)
	id := newSession(t, a, "BUG-1234", false)
	statePath, endRun := recordedShell(t, a, dir, id, "c session end -m done")
	out, _, err := runEmpty(t, a.sessionEndCmd(), nil, map[string]string{"note": "done"})
	if err != nil {
		t.Fatalf("end: %v", err)
	}
	state := (&carriershell.StateFile{Path: statePath}).Read()
	if state.SessionID == id || state.SessionID == 0 {
		t.Fatalf("the shell should move to a new session, state = %+v", state)
	}
	if !strings.Contains(out, fmt.Sprintf("now records into session %d", state.SessionID)) {
		t.Fatalf("output = %q", out)
	}
	next, err := a.st.GetSession(state.SessionID)
	if err != nil || !next.Auto || next.Label != "" {
		t.Fatalf("new session = %+v, %v", next, err)
	}
	if info, _ := os.ReadFile(carriershell.SessionInfoPath(statePath)); string(info) != fmt.Sprintf("%d\n\n", state.SessionID) {
		t.Fatalf("session info = %q", info)
	}
	if _, err := a.st.GetRun(endRun); err == nil {
		t.Fatal("the end command's own run should be dropped")
	}
	if sess, _ := a.st.GetSession(id); sess.EndedAt == nil {
		t.Fatal("the session should be ended")
	}
}

func TestAskOutcome(t *testing.T) {
	a, _ := autoSessionApp(t)
	labeled := newSession(t, a, "OPS-7: deploy", false)
	unlabeled := newSession(t, a, "", true)

	var out strings.Builder
	if err := a.askOutcome(labeled, strings.NewReader("deploy key had expired\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `what did "OPS-7: deploy" find?`) {
		t.Fatalf("prompt = %q", out.String())
	}
	if o, _ := a.st.SessionOutcome(labeled); o == nil || o.Text != "deploy key had expired" {
		t.Fatalf("outcome = %+v", o)
	}

	out.Reset()
	if err := a.askOutcome(labeled, strings.NewReader("again\n"), &out); err != nil || out.Len() != 0 {
		t.Fatalf("a session with an outcome should not be asked again: %q %v", out.String(), err)
	}
	if err := a.askOutcome(unlabeled, strings.NewReader("x\n"), &out); err != nil || out.Len() != 0 {
		t.Fatalf("unlabeled sessions should not be asked: %q %v", out.String(), err)
	}
	skip := newSession(t, a, "skip me", false)
	if err := a.askOutcome(skip, strings.NewReader("\n"), &out); err != nil {
		t.Fatal(err)
	}
	if o, _ := a.st.SessionOutcome(skip); o != nil {
		t.Fatal("Enter should skip")
	}
}

func TestDisplayOutputAndMarkdownHelpers(t *testing.T) {
	if got := displayOutput("\x1b]11;?\x1b\\\x1b[32mok\x1b[0m\r\n10%\r50%\r100%\n"); got != "ok\n100%\n" {
		t.Errorf("displayOutput = %q", got)
	}
	for in, want := range map[string]string{"make": "`make`", "echo `x`": "`` echo `x` ``", "`x": "`` `x ``"} {
		if got := mdCode(in); got != want {
			t.Errorf("mdCode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := mdFence([]string{"a ``` b"}); got != "````" {
		t.Errorf("mdFence = %q", got)
	}
	if got := formatSpan(80 * time.Minute); got != "1h 20m" {
		t.Errorf("formatSpan = %q", got)
	}
}
