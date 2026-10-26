package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestParseReportPeriod(t *testing.T) {
	now := time.Date(2026, 10, 26, 18, 30, 0, 0, time.Local)
	midnight := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)
	cases := []struct {
		arg, since string
		name       string
		from, to   time.Time
	}{
		{"", "", "today", midnight, now},
		{"today", "", "today", midnight, now},
		{"yesterday", "", "yesterday", midnight.AddDate(0, 0, -1), midnight},
		{"week", "", "the last 7 days", midnight.AddDate(0, 0, -6), now},
		{"2026-10-20", "", "Tuesday 20 October 2026", time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)},
		{"", "3h", "the last 3h", now.Add(-3 * time.Hour), now},
	}
	for _, c := range cases {
		p, err := parseReportPeriod(c.arg, c.since, now)
		if err != nil {
			t.Fatalf("%q --since %q: %v", c.arg, c.since, err)
		}
		if p.Name != c.name || !p.From.Equal(c.from) || !p.To.Equal(c.to) {
			t.Errorf("%q --since %q = %q %v–%v, want %q %v–%v", c.arg, c.since, p.Name, p.From, p.To, c.name, c.from, c.to)
		}
	}
	for _, bad := range []struct{ arg, since, want string }{
		{"tomorrow", "", "unknown period"},
		{"week", "2d", "not both"},
		{"", "soon", "invalid --since"},
	} {
		if _, err := parseReportPeriod(bad.arg, bad.since, now); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%q --since %q: err = %v, want %q", bad.arg, bad.since, err, bad.want)
		}
	}
}

// reportFixture is a day of made-up work: a labeled session in the api
// repository with a note and an outcome, automatic sessions there, runs
// outside sessions in a scratch directory, and runs from the day before.
type reportFixture struct {
	st                  *store.Store
	a                   *app
	api                 string
	login, auto1, auto2 int64
	stillFailing        int64
}

func newReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 10, 26, 18, 0, 0, 0, time.Local)
	old := reportNow
	reportNow = func() time.Time { return now }
	t.Cleanup(func() { reportNow = old })

	f := &reportFixture{st: st, a: &app{st: st}, api: filepath.Join(t.TempDir(), "api")}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 26, h, m, 0, 0, time.Local) }
	session := func(label string, auto bool) int64 {
		id, err := st.CreateSession(store.CreateSession{Label: label, StartedAt: at(9, 0), Auto: auto})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.login = session("BUG-1234: fix login", false)
	f.auto1 = session("", true)
	f.auto2 = session("", true)
	run := func(command, cwd, root, branch string, sess int64, started time.Time, status string) int64 {
		t.Helper()
		r := store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: command, ArgvJSON: "[]",
			CWD: cwd, GitRoot: root, GitBranch: branch, StartedAt: started,
		}
		if sess != 0 {
			r.SessionID = &sess
		}
		id, err := st.CreateRun(r)
		if err != nil {
			t.Fatal(err)
		}
		exit := 0
		if status == store.StatusFailed {
			exit = 1
		}
		if err := st.FinishRun(id, status, exit, started.Add(90*time.Second)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	api, scratch := f.api, "/srv/scratch"
	ok, failed := store.StatusSuccess, store.StatusFailed
	run("go test ./...", api, api, "fix-login", f.login, at(9, 10), failed)
	run("go test ./...", api, api, "fix-login", f.login, at(9, 40), ok)
	f.stillFailing = run("make e2e", api, api, "fix-login", f.login, at(10, 5), failed)
	run("go test ./...", api, api, "fix-login", f.login, at(10, 20), ok)
	run("git status", api, api, "main", f.auto1, at(13, 0), ok)
	run("git pull", api, api, "main", f.auto2, at(14, 0), ok)
	run("ls", scratch, "", "", 0, at(15, 0), ok)
	run("du -sh .", scratch, "", "", 0, at(15, 5), ok)
	// The day before, left out of today's report.
	run("make release", api, api, "main", 0, at(9, 0).AddDate(0, 0, -1), failed)

	note := func(text, kind string, sess int64, when time.Time) {
		t.Helper()
		if _, err := st.CreateNote(store.CreateNote{SessionID: &sess, Text: text, Kind: kind, CreatedAt: when}); err != nil {
			t.Fatal(err)
		}
	}
	note("token refresh races the logout", "", f.login, at(9, 50))
	note("fixed by locking the refresh", store.NoteOutcome, f.login, at(10, 30))
	return f
}

func TestReportText(t *testing.T) {
	f := newReportFixture(t)
	out, _, err := runEmpty(t, f.a.reportCmd(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Today, Monday 26 October 2026",
		"8 runs in 3 sessions, 2 failed, 12m in commands.",
		"api  " + collapseHome(f.api) + " · fix-login, main",
		"09:10–14:00 · 6 runs, 2 failed",
		"• BUG-1234: fix login (session 1) · 09:10–10:20 · 4 runs, 2 failed",
		"09:50 token refresh races the logout",
		"outcome: fixed by locking the refresh",
		"• 2 other sessions · 2 runs",
		"most run: go test ./... ×3",
		"✗ still failing: make e2e  run 3, 10:05",
		"/srv/scratch\n  15:00–15:05 · 2 runs\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"make release", "outside sessions", "ls ×"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("report should not show %q:\n%s", unwanted, out)
		}
	}
	// The place with the most runs comes first.
	if strings.Index(out, "api  ") > strings.Index(out, "/srv/scratch") {
		t.Errorf("api should come before scratch:\n%s", out)
	}
}

func TestReportYesterdayAndEmpty(t *testing.T) {
	f := newReportFixture(t)
	out, _, err := runEmpty(t, f.a.reportCmd(), []string{"yesterday"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Yesterday, Sunday 25 October 2026") || !strings.Contains(out, "1 run, 1 failed") || !strings.Contains(out, "still failing: make release") {
		t.Errorf("yesterday:\n%s", out)
	}
	out, _, err = runEmpty(t, f.a.reportCmd(), []string{"2026-10-01"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No commands recorded on Thursday 1 October 2026.") {
		t.Errorf("empty day:\n%s", out)
	}
}

func TestReportMarkdown(t *testing.T) {
	f := newReportFixture(t)
	out, _, err := runEmpty(t, f.a.reportCmd(), nil, map[string]string{"markdown": "true"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Today, Monday 26 October 2026\n\n8 runs in 3 sessions, 2 failed, 12m in commands.\n",
		"### api\n\n`" + collapseHome(f.api) + "` · on `fix-login`, `main` · 6 runs, 2 failed, 09:10–14:00\n",
		"- **BUG-1234: fix login** (session 1): 4 runs, 2 failed, 09:10–10:20\n",
		"  - Outcome: fixed by locking the refresh\n",
		"  - Note: token refresh races the logout\n",
		"- 2 other sessions: 2 runs\n",
		"- Most run: `go test ./...` ×3\n",
		"- Still failing: `make e2e`\n",
		"### /srv/scratch\n\n2 runs, 15:00–15:05\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown is missing %q:\n%s", want, out)
		}
	}
}

func TestReportJSON(t *testing.T) {
	f := newReportFixture(t)
	out, _, err := runEmpty(t, f.a.reportCmd(), nil, map[string]string{"json": "true"})
	if err != nil {
		t.Fatal(err)
	}
	var v reportView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if v.Period != "today" || v.Runs != 8 || v.Failed != 2 || v.Sessions != 3 || len(v.Places) != 2 {
		t.Fatalf("report = %+v", v)
	}
	api := v.Places[0]
	if api.Path != f.api || !api.Repo || len(api.Sessions) != 3 || api.Outside != nil {
		t.Fatalf("api = %+v", api)
	}
	login := api.Sessions[0]
	if login.ID != f.login || login.Runs != 4 || login.Outcome != "fixed by locking the refresh" || len(login.Notes) != 1 {
		t.Errorf("login session = %+v", login)
	}
	if len(api.StillFailing) != 1 || api.StillFailing[0].ID != f.stillFailing {
		t.Errorf("still failing = %+v", api.StillFailing)
	}
	if scratch := v.Places[1]; scratch.Repo || scratch.Outside == nil || scratch.Outside.Runs != 2 || len(scratch.Sessions) != 0 {
		t.Errorf("scratch = %+v", scratch)
	}
}

func TestReportRepoAndNotesFromEarlierSessions(t *testing.T) {
	f := newReportFixture(t)
	// A session from last week, written up today: its note is reported where
	// it ran commands.
	sess, err := f.st.CreateSession(store.CreateSession{Label: "BUG-99: flaky deploy", StartedAt: time.Date(2026, 10, 19, 9, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatal(err)
	}
	id, err := f.st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeShell, Command: "deploy", ArgvJSON: "[]", CWD: f.api, GitRoot: f.api,
		StartedAt: time.Date(2026, 10, 19, 9, 0, 0, 0, time.Local), SessionID: &sess,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.st.FinishRun(id, store.StatusSuccess, 0, time.Date(2026, 10, 19, 9, 1, 0, 0, time.Local))
	if _, err := f.st.CreateNote(store.CreateNote{SessionID: &sess, Text: "retries fixed it", CreatedAt: time.Date(2026, 10, 26, 11, 0, 0, 0, time.Local)}); err != nil {
		t.Fatal(err)
	}

	fakeRepo(t, filepath.Dir(f.api), "main")
	// fakeRepo makes the repository at <dir>/repo; record runs there too.
	repo := filepath.Join(filepath.Dir(f.api), "repo")
	if _, err := f.st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "make", ArgvJSON: "[]", CWD: repo, GitRoot: repo,
		StartedAt: time.Date(2026, 10, 26, 16, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runEmpty(t, f.a.reportCmd(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "• BUG-99: flaky deploy (session 4)\n      11:00 retries fixed it\n") {
		t.Errorf("the note on last week's session is missing:\n%s", out)
	}

	out, _, err = runEmpty(t, f.a.reportCmd(), nil, map[string]string{"repo": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 run.") || strings.Contains(out, collapseHome(f.api)) || strings.Contains(out, "retries fixed it") {
		t.Errorf("--repo should only report the repository here:\n%s", out)
	}
}

func TestMarkdownEscaping(t *testing.T) {
	if got := markdownText("fix *all* the [things] #1"); got != `fix \*all\* the \[things\] \#1` {
		t.Errorf("markdownText = %q", got)
	}
	if got := markdownCode("echo `date`"); got != "`` echo `date` ``" {
		t.Errorf("markdownCode = %q", got)
	}
	if got := markdownCode("go test"); got != "`go test`" {
		t.Errorf("markdownCode = %q", got)
	}
}

func TestRoughDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		35 * time.Second:                      "35s",
		4*time.Minute + 20*time.Second:        "4m",
		2*time.Hour + 10*time.Minute:          "2h 10m",
		3*time.Hour + 20*time.Second:          "3h",
		59*time.Minute + 50*time.Second:       "1h",
		59*time.Second + 600*time.Millisecond: "1m",
	} {
		if got := roughDuration(d); got != want {
			t.Errorf("roughDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
