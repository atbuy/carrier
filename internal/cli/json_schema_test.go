package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

var updateJSONSchema = flag.Bool("update-json-schema", false, "rewrite testdata/json-schema.golden")

// TestJSONOutputSchema pins the field names and types of every --json output.
// Renaming or removing a field breaks scripts, which docs/reference/compatibility.md
// says 1.x avoids. Adding a field is fine: rerun with -update-json-schema and
// mention the new field in CHANGELOG.md.
func TestJSONOutputSchema(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	ids := seedJSONSchemaStore(t, st)
	a := &app{st: st}

	full := fmt.Sprint(ids.full)
	cases := []struct {
		name  string
		cmd   func() *cobra.Command
		args  []string
		flags map[string]string
		setup func()
	}{
		{"last", a.lastCmd, nil, map[string]string{"json": "true"}, nil},
		{"last N", a.lastCmd, []string{"3"}, map[string]string{"json": "true"}, nil},
		{"show", a.showCmd, []string{full}, map[string]string{"json": "true"}, nil},
		{"export --format json", a.exportCmd, []string{full}, map[string]string{"format": "json"}, nil},
		{"history", a.historyCmd, nil, map[string]string{"json": "true"}, nil},
		{"history --sessions-only", a.historyCmd, nil, map[string]string{"json": "true", "sessions-only": "true"}, nil},
		{"failed", a.failedCmd, nil, map[string]string{"json": "true"}, nil},
		{"running", a.runningCmd, nil, map[string]string{"json": "true"}, nil},
		{"search", a.searchCmd, []string{"refused"}, map[string]string{"json": "true"}, nil},
		{"similar", a.similarCmd, []string{full}, map[string]string{"json": "true"}, nil},
		{"stats", a.statsCmd, nil, map[string]string{"json": "true"}, nil},
		{"notes", a.notesCmd, nil, map[string]string{"json": "true"}, nil},
		{"session list", a.sessionListCmd, nil, map[string]string{"json": "true"}, nil},
		{"session show", a.sessionShowCmd, []string{"1234"}, map[string]string{"json": "true", "output": "true"}, nil},
		{"session export --format json", a.sessionExportCmd, []string{"1234"}, map[string]string{"format": "json", "output": "true"}, nil},
		{"changelog", a.changelogCmd, []string{"0.24.0"}, map[string]string{"json": "true"}, nil},
	}

	// The status setup changes directory, so find the golden file first.
	golden, err := filepath.Abs(filepath.Join("testdata", "json-schema.golden"))
	if err != nil {
		t.Fatal(err)
	}
	// status runs last: it needs a recorded shell, which adds a run.
	cases = append(cases, struct {
		name  string
		cmd   func() *cobra.Command
		args  []string
		flags map[string]string
		setup func()
	}{"status", a.statusCmd, nil, map[string]string{"json": "true"}, func() {
		dir := t.TempDir()
		t.Chdir(dir)
		t.Setenv("TMUX", "")
		if err := os.WriteFile(pinFileName, []byte("BUG-1234\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		recordedShell(t, a, dir, 1, "make test")
		// Not a terminal in tests, so this does not make it another terminal.
		t.Setenv("CARRIER_TTY", "/dev/pts/7")
	}})

	var got strings.Builder
	for _, tc := range cases {
		if tc.setup != nil {
			tc.setup()
		}
		out, _, err := runEmpty(t, tc.cmd(), tc.args, tc.flags)
		if err != nil {
			t.Fatalf("%s --json: %v", tc.name, err)
		}
		var value any
		if err := json.Unmarshal([]byte(out), &value); err != nil {
			t.Fatalf("%s: invalid JSON: %v\n%s", tc.name, err, out)
		}
		if arr, ok := value.([]any); ok && len(arr) == 0 {
			t.Fatalf("%s: empty array; seed data so every field is present", tc.name)
		}
		fmt.Fprintf(&got, "## %s\n", tc.name)
		for _, line := range jsonSchema(value) {
			fmt.Fprintln(&got, line)
		}
		got.WriteString("\n")
	}

	if *updateJSONSchema {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v (run go test ./internal/cli -run TestJSONOutputSchema -update-json-schema)", golden, err)
	}
	if got.String() == string(want) {
		return
	}
	removed, added := diffLines(string(want), got.String())
	var msg strings.Builder
	msg.WriteString("--json output changed.\n")
	if len(removed) > 0 {
		msg.WriteString("\nRemoved or changed (breaks scripts; see docs/reference/compatibility.md):\n  " + strings.Join(removed, "\n  ") + "\n")
	}
	if len(added) > 0 {
		msg.WriteString("\nAdded (fine; rerun with -update-json-schema and note it in CHANGELOG.md):\n  " + strings.Join(added, "\n  ") + "\n")
	}
	t.Fatal(msg.String())
}

// jsonMapFields are objects keyed by data, such as variable names, rather
// than by field names.
var jsonMapFields = map[string]bool{"env": true}

// jsonSchema lists every field path in v with its JSON type, such as
// "[].notes[].text string". Array elements are merged, values are ignored,
// and the values of jsonMapFields are listed as "env{} string".
func jsonSchema(v any) []string {
	set := map[string]bool{}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if path == "" {
				set["(root) object"] = true
			} else {
				set[path+" object"] = true
			}
			if jsonMapFields[strings.TrimPrefix(path, "[].")] {
				for _, child := range x {
					walk(path+"{}", child)
				}
				return
			}
			for k, child := range x {
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(p, child)
			}
		case []any:
			if path == "" {
				set["(root) array"] = true
			} else {
				set[path+" array"] = true
			}
			for _, child := range x {
				walk(path+"[]", child)
			}
		case string:
			set[path+" string"] = true
		case float64:
			set[path+" number"] = true
		case bool:
			set[path+" bool"] = true
		case nil:
			set[path+" null"] = true
		}
	}
	walk("", v)
	lines := make([]string, 0, len(set))
	for line := range set {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}

// diffLines reports the "## section" qualified lines only in want (removed)
// and only in got (added).
func diffLines(want, got string) (removed, added []string) {
	qualify := func(text string) []string {
		var lines []string
		section := ""
		for _, line := range strings.Split(text, "\n") {
			switch {
			case strings.HasPrefix(line, "## "):
				section = strings.TrimPrefix(line, "## ")
			case line != "":
				lines = append(lines, section+": "+line)
			}
		}
		return lines
	}
	w, g := qualify(want), qualify(got)
	for _, line := range w {
		if !slices.Contains(g, line) {
			removed = append(removed, line)
		}
	}
	for _, line := range g {
		if !slices.Contains(w, line) {
			added = append(added, line)
		}
	}
	return removed, added
}

type jsonSchemaIDs struct{ full int64 }

// seedJSONSchemaStore records runs, a session, and notes with every optional
// field set, so omitempty fields show up in the schema.
func seedJSONSchemaStore(t *testing.T, st *store.Store) jsonSchemaIDs {
	t.Helper()
	dir := t.TempDir()
	writeLog := func(name, text string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// A fixed time, so the data does not depend on when the test runs.
	started := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
	sessID, err := st.CreateSession(store.CreateSession{Label: "BUG-1234: fix login", StartedAt: started, Auto: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionKept(sessID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.EndSession(sessID, started.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	envID, err := st.InsertOrGetEnvironment(`{"HOME":"/home/dev","LANG":"C.UTF-8"}`)
	if err != nil {
		t.Fatal(err)
	}
	dirty := true
	failedRun := func(n int, label string) int64 {
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: "psql -h db",
			ArgvJSON: `["psql","-h","db"]`, CWD: "/home/dev/api", Label: label,
			StartedAt: started.Add(time.Duration(n) * time.Minute), Hostname: "laptop", Shell: "/bin/zsh",
			GitRoot: "/home/dev/api", GitBranch: "main", GitCommit: "abc1234", GitDirty: &dirty,
			StdoutPath:         writeLog(fmt.Sprintf("%d.stdout.log", n), "connecting\n"),
			StderrPath:         writeLog(fmt.Sprintf("%d.stderr.log", n), "error: connection refused\n"),
			TerminalOutputPath: writeLog(fmt.Sprintf("%d.terminal.log", n), "error: connection refused\n"),
			NotifyRequested:    true, NotifyAlways: true, EnvID: &envID, SessionID: &sessID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.FinishRun(id, store.StatusFailed, 2, started.Add(time.Duration(n)*time.Minute+time.Second)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	if _, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeRun, Command: "npm start",
		ArgvJSON: `["npm","start"]`, CWD: "/home/dev/web", StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	earlier := failedRun(1, "db down")
	full := failedRun(2, "db down again")
	for _, n := range []store.CreateNote{
		{RunID: &earlier, SessionID: &sessID, Text: "the database was restarting"},
		{RunID: &full, SessionID: &sessID, Text: "still down after the restart"},
		{SessionID: &sessID, Text: "login fails because the database is down"},
		{SessionID: &sessID, Text: "the pool size was 0", Kind: store.NoteOutcome},
	} {
		n.CreatedAt = started.Add(time.Hour)
		if _, err := st.CreateNote(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{earlier, full} {
		if err := st.ReindexRun(id); err != nil {
			t.Fatal(err)
		}
	}
	return jsonSchemaIDs{full: full}
}
