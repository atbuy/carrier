package store

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFileURI(t *testing.T) {
	if got, want := fileURI("/home/a b/carrier.db", "mode=ro"), "file:///home/a%20b/carrier.db?mode=ro"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// filepath.ToSlash only converts backslashes on Windows, so a drive path
	// with forward slashes stands in for one here.
	if got, want := fileURI("C:/Users/dev/carrier.db", "mode=ro"), "file:///C:/Users/dev/carrier.db?mode=ro"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOpenReadOnlyNeedsAnExistingStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "carrier")
	if _, err := OpenReadOnly(dir); err == nil {
		t.Fatal("OpenReadOnly should fail when there is no database")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created %s; completion must not write anything", dir)
	}
}

func TestOpenReadOnlyCannotWrite(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	if _, err := ro.CreateSession(CreateSession{StartedAt: time.Now()}); err == nil {
		t.Fatal("a read-only store accepted a write")
	}
}

// completionStore records runs and sessions for the completion queries.
func completionStore(t *testing.T) (*Store, map[string]int64) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	start := time.Date(2025, 1, 2, 15, 0, 0, 0, time.UTC)
	ids := map[string]int64{}
	session := func(name, label string, auto, kept bool) {
		id, err := st.CreateSession(CreateSession{Label: label, StartedAt: start, Auto: auto})
		if err != nil {
			t.Fatal(err)
		}
		if kept {
			if err := st.SetSessionKept(id, true); err != nil {
				t.Fatal(err)
			}
		}
		ids[name] = id
	}
	session("bug", "BUG-1234: fix login", false, false)
	session("kept", "api refactor", false, true)
	session("auto", "", true, false)
	session("empty", "", true, false)
	for i, r := range []struct{ name, cmd, label, status, branch, session string }{
		{"first", "go test ./...", "", StatusFailed, "main", "bug"},
		{"deploy", "kubectl apply -f k8s", "deploy prod", StatusSuccess, "main", "kept"},
		{"odd", "echo 100%", "50% done_x", StatusSuccess, "BUG-1234-fix", "bug"},
		{"running", "npm run dev", "", StatusRunning, "BUG-1234-fix", "auto"},
		{"again", "go test ./...", "", StatusSuccess, "BUG-1234-fix", "bug"},
	} {
		sess := ids[r.session]
		id, err := st.CreateRun(CreateRun{
			Status: StatusRunning, Mode: ModeShell, Command: r.cmd, ArgvJSON: "[]", CWD: "/home/dev/" + r.branch,
			Label: r.label, StartedAt: start.Add(time.Duration(i) * time.Minute), GitBranch: r.branch, SessionID: &sess,
		})
		if err != nil {
			t.Fatal(err)
		}
		if r.status != StatusRunning {
			if err := st.FinishRun(id, r.status, 0, start.Add(time.Duration(i)*time.Minute+time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		ids[r.name] = id
	}
	return st, ids
}

func runIDs(choices []RunChoice) []int64 {
	out := make([]int64, len(choices))
	for i, c := range choices {
		out[i] = c.ID
	}
	return out
}

func TestRunChoices(t *testing.T) {
	st, ids := completionStore(t)
	for _, tc := range []struct {
		name string
		f    RunChoiceFilter
		want []int64
	}{
		{"newest first", RunChoiceFilter{}, []int64{ids["again"], ids["running"], ids["odd"], ids["deploy"], ids["first"]}},
		{"limit", RunChoiceFilter{Limit: 2}, []int64{ids["again"], ids["running"]}},
		{"ID prefix", RunChoiceFilter{IDPrefix: fmt.Sprint(ids["deploy"])}, []int64{ids["deploy"]}},
		{"label prefix ignores case", RunChoiceFilter{LabelPrefix: "DEP"}, []int64{ids["deploy"]}},
		{"label prefix is literal", RunChoiceFilter{LabelPrefix: "50%"}, []int64{ids["odd"]}},
		{"no wildcard match", RunChoiceFilter{LabelPrefix: "5_%"}, nil},
		{"running only", RunChoiceFilter{Running: true}, []int64{ids["running"]}},
		{"exclude", RunChoiceFilter{Exclude: []int64{ids["again"], ids["running"]}, Limit: 1}, []int64{ids["odd"]}},
	} {
		got, err := st.RunChoices(tc.f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(runIDs(got), tc.want) {
			t.Errorf("%s: got runs %v, want %v", tc.name, runIDs(got), tc.want)
		}
	}
	got, _ := st.RunChoices(RunChoiceFilter{IDPrefix: fmt.Sprint(ids["deploy"])})
	if c := got[0]; c.Status != StatusSuccess || c.Command != "kubectl apply -f k8s" || c.Label != "deploy prod" || c.StartedAt.IsZero() {
		t.Errorf("choice = %+v", c)
	}
}

func TestSessionChoices(t *testing.T) {
	st, ids := completionStore(t)
	sessionIDs := func(f SessionChoiceFilter) []int64 {
		t.Helper()
		got, err := st.SessionChoices(f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]int64, len(got))
		for i, c := range got {
			out[i] = c.ID
		}
		return out
	}
	kept, notKept := true, false
	// The empty automatic session has nothing to offer and is left out.
	if got, want := sessionIDs(SessionChoiceFilter{}), []int64{ids["auto"], ids["kept"], ids["bug"]}; !slices.Equal(got, want) {
		t.Errorf("all: got %v, want %v", got, want)
	}
	if got, want := sessionIDs(SessionChoiceFilter{Kept: &kept}), []int64{ids["kept"]}; !slices.Equal(got, want) {
		t.Errorf("kept: got %v, want %v", got, want)
	}
	if got, want := sessionIDs(SessionChoiceFilter{Kept: &notKept, Exclude: []int64{ids["auto"]}}), []int64{ids["bug"]}; !slices.Equal(got, want) {
		t.Errorf("not kept, excluding auto: got %v, want %v", got, want)
	}
	if got, want := sessionIDs(SessionChoiceFilter{LabelPrefix: "bug-1234:"}), []int64{ids["bug"]}; !slices.Equal(got, want) {
		t.Errorf("label: got %v, want %v", got, want)
	}
	got, _ := st.SessionChoices(SessionChoiceFilter{LabelPrefix: "BUG"})
	if c := got[0]; c.Runs != 3 || c.Kept || c.Ended || c.Label != "BUG-1234: fix login" {
		t.Errorf("choice = %+v", c)
	}
}

func TestRunValues(t *testing.T) {
	st, _ := completionStore(t)
	for _, tc := range []struct {
		col    RunValueColumn
		prefix string
		want   []string
	}{
		// Most recently used first.
		{ValueBranch, "", []string{"BUG-1234-fix", "main"}},
		{ValueBranch, "ma", []string{"main"}},
		{ValueLabel, "", []string{"50% done_x", "deploy prod"}},
		{ValueCWD, "/home/dev/m", []string{"/home/dev/main"}},
		// Most used first.
		{ValueCommand, "", []string{"go test ./...", "npm run dev", "echo 100%", "kubectl apply -f k8s"}},
	} {
		got, err := st.RunValues(tc.col, tc.prefix, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s %q: got %q, want %q", tc.col, tc.prefix, got, tc.want)
		}
	}
	if _, err := st.RunValues("status; DROP TABLE runs", "", 0); err == nil {
		t.Fatal("RunValues accepted an unknown column")
	}
}

func TestNoteChoices(t *testing.T) {
	st, ids := completionStore(t)
	sess := ids["bug"]
	var noteIDs []int64
	for _, text := range []string{"first note", "second\nnote"} {
		id, err := st.CreateNote(CreateNote{SessionID: &sess, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		noteIDs = append(noteIDs, id)
	}
	got, err := st.NoteChoices("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != noteIDs[1] || got[1].Text != "first note" {
		t.Fatalf("notes = %+v", got)
	}
}

// TestCompletionQueriesAreFast checks that a Tab press stays quick on a large
// history: each completion query must finish well within the time a shell
// user would notice.
func TestCompletionQueriesAreFast(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a large store")
	}
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const runs, sessions = 20000, 400
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= sessions; i++ {
		if _, err := tx.Exec(`INSERT INTO shell_sessions (label, started_at, auto, kept) VALUES (?, ?, ?, 0)`,
			fmt.Sprintf("session %d", i), fmtTime(time.Now()), i%2); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= runs; i++ {
		label := ""
		if i%10 == 0 {
			label = fmt.Sprintf("label %d", i)
		}
		if _, err := tx.Exec(`INSERT INTO runs (status, mode, command, argv_json, cwd, label, started_at, git_branch, session_id, hostname, shell, git_root, git_commit, stdout_path, stderr_path, terminal_output_path)
VALUES (?, 'shell', ?, '[]', ?, ?, ?, ?, ?, 'devbox', 'zsh', '', '', '', '', '')`, StatusSuccess, fmt.Sprintf("make target-%d", i%300), fmt.Sprintf("/home/dev/p%d", i%50),
			nullable(label), fmtTime(time.Now()), fmt.Sprintf("branch-%d", i%80), i%sessions+1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	start := time.Now()
	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	kept := false
	queries := map[string]func() error{
		"runs":           func() error { _, err := ro.RunChoices(RunChoiceFilter{}); return err },
		"runs by ID":     func() error { _, err := ro.RunChoices(RunChoiceFilter{IDPrefix: "19"}); return err },
		"runs by label":  func() error { _, err := ro.RunChoices(RunChoiceFilter{LabelPrefix: "label 1"}); return err },
		"running runs":   func() error { _, err := ro.RunChoices(RunChoiceFilter{Running: true}); return err },
		"sessions":       func() error { _, err := ro.SessionChoices(SessionChoiceFilter{}); return err },
		"unkept":         func() error { _, err := ro.SessionChoices(SessionChoiceFilter{Kept: &kept}); return err },
		"branches":       func() error { _, err := ro.RunValues(ValueBranch, "", 0); return err },
		"commands":       func() error { _, err := ro.RunValues(ValueCommand, "make", 0); return err },
		"directories":    func() error { _, err := ro.RunValues(ValueCWD, "", 0); return err },
		"notes by ID":    func() error { _, err := ro.NoteChoices("1", 0); return err },
		"labels by text": func() error { _, err := ro.RunValues(ValueLabel, "label", 0); return err },
		"history search": func() error { _, err := ro.RecentCommands(5000); return err },
	}
	for name, query := range queries {
		begin := time.Now()
		if err := query(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d := time.Since(begin); d > 150*time.Millisecond {
			t.Errorf("%s took %v on %d runs", name, d, runs)
		} else {
			t.Logf("%s: %v", name, d)
		}
	}
	t.Logf("open and every query: %v", time.Since(start))
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestRecentCommands(t *testing.T) {
	st, ids := completionStore(t)
	got, err := st.RecentCommands(0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i, c := range got {
		if seen[c.Command] {
			t.Errorf("%q is listed twice", c.Command)
		}
		seen[c.Command] = true
		if i > 0 && c.ID > got[i-1].ID {
			t.Errorf("not newest first: %d after %d", c.ID, got[i-1].ID)
		}
		if c.Command == "go test ./..." && (c.ID != ids["again"] || c.Runs != 2 || c.Status != StatusSuccess) {
			t.Errorf("go test should show its newest run, %d, and 2 runs: %+v", ids["again"], c)
		}
	}
	if !seen["go test ./..."] || !seen["kubectl apply -f k8s"] {
		t.Fatalf("commands = %v", seen)
	}
	if limited, _ := st.RecentCommands(2); len(limited) != 2 || limited[0].ID != got[0].ID || limited[1].ID != got[1].ID {
		t.Fatalf("limit 2 should keep the newest two: got %+v", limited)
	}
}

// TestRecentCommandsUsesCommandIndex keeps history search quick on a long
// history: grouping the commands must read the index, not every run.
func TestRecentCommandsUsesCommandIndex(t *testing.T) {
	st, _ := completionStore(t)
	rows, err := st.db.Query(`EXPLAIN QUERY PLAN SELECT MAX(id) AS id, COUNT(*) AS n FROM runs WHERE TRIM(command) != '' GROUP BY command ORDER BY 1 DESC LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "idx_runs_command") {
		t.Fatalf("grouping does not use idx_runs_command: %v", plan)
	}
}
