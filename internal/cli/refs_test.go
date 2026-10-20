package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func TestPickLabel(t *testing.T) {
	// Newest first, as the store returns them.
	cands := []labelCandidate{
		{ID: 9, Label: "deploy"},
		{ID: 8, Label: "OPS-1234 rotate keys"},
		{ID: 7, Label: "BUG-1234: Doing something"},
		{ID: 5, Label: "deploy"},
		{ID: 4, Label: "BUG-12345 other"},
		{ID: 3, Label: "Backend debug"},
	}
	cases := []struct {
		query string
		want  int64  // 0 when an error is expected
		err   string // substring of the expected error
	}{
		{"deploy", 9, ""},                    // exact, newest of the same label
		{"DEPLOY", 9, ""},                    // case-insensitive
		{"BUG-1234: Doing something", 7, ""}, // exact full label
		{"bug-1234", 7, ""},                  // whole words
		{"BUG 1234", 7, ""},                  // punctuation does not matter
		{"1234 doing", 7, ""},                // words in order
		{"12345", 4, ""},                     // does not match 1234
		{"debug", 3, ""},                     // single word
		{"doing some", 7, ""},                // unique substring
		{"1234", 0, `"1234" matches several run labels: run 8 "OPS-1234 rotate keys", run 7 "BUG-1234: Doing something"`},
		{"123", 0, "matches several run labels"}, // no whole word; substring of different labels
		{"missing", 0, "no label matches"},
	}
	for _, tc := range cases {
		got, err := pickLabel("run", tc.query, cands)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("pickLabel(%q) err = %v, want %q", tc.query, err, tc.err)
			}
			continue
		}
		if err != nil || got.ID != tc.want {
			t.Errorf("pickLabel(%q) = %d, %v; want %d", tc.query, got.ID, err, tc.want)
		}
	}
	if _, err := pickLabel("run", "x", nil); err != errNoLabelMatch {
		t.Fatalf("no candidates: err = %v", err)
	}
}

func TestLabelWordsAndContainsWords(t *testing.T) {
	if got := strings.Join(labelWords("BUG-1234: Doing_something/else"), ","); got != "bug,1234,doing,something,else" {
		t.Fatalf("labelWords = %q", got)
	}
	words := []string{"bug", "1234", "doing", "something"}
	for want, ok := range map[string]bool{"bug 1234": true, "1234 doing": true, "doing bug": false, "something else": false} {
		if got := containsWords(words, strings.Fields(want)); got != ok {
			t.Errorf("containsWords(%q) = %v, want %v", want, got, ok)
		}
	}
}

func TestRewriteRunRefArgs(t *testing.T) {
	root := &cobra.Command{Use: "carrier"}
	root.PersistentFlags().BoolP("quiet", "q", false, "")
	for _, name := range []string{"show", "diff", "run", "note", "notes", "history"} {
		c := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}}
		if name == "show" {
			c.Aliases = []string{"sh"}
		}
		if name == "notes" {
			c.AddCommand(&cobra.Command{Use: "rm", Run: func(*cobra.Command, []string) {}})
		}
		root.AddCommand(c)
	}
	rel := func(n string) string { return relativeRunRef + n }
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"show", "-1"}, []string{"show", rel("1")}},
		{[]string{"sh", "-2"}, []string{"sh", rel("2")}},
		{[]string{"-q", "show", "-3", "--json"}, []string{"-q", "show", rel("3"), "--json"}},
		{[]string{"diff", "-2", "-1"}, []string{"diff", rel("2"), rel("1")}},
		{[]string{"show", "--lines", "-1"}, []string{"show", "--lines", "-1"}},
		{[]string{"run", "head", "-1", "f"}, []string{"run", "head", "-1", "f"}},
		{[]string{"note", "-r", "-2", "hung"}, []string{"note", "-r", rel("2"), "hung"}},
		{[]string{"note", "--run=-2", "hung"}, []string{"note", "--run=" + rel("2"), "hung"}},
		{[]string{"note", "off", "by", "-1"}, []string{"note", "off", "by", "-1"}},
		{[]string{"notes", "rm", "-1"}, []string{"notes", "rm", "-1"}},
		{[]string{"history", "-1"}, []string{"history", "-1"}},
		{[]string{"unknown", "-1"}, []string{"unknown", "-1"}},
		{[]string{}, []string{}},
	}
	for _, tc := range cases {
		got := rewriteRunRefArgs(root, tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("rewriteRunRefArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if displayRef(rel("4")) != "-4" || displayRef("deploy") != "deploy" {
		t.Fatal("displayRef should show references as typed")
	}
}

func createLabeledRun(t *testing.T, st *store.Store, label string) int64 {
	t.Helper()
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeRun, Command: "echo", ArgvJSON: `["echo"]`,
		CWD: "/tmp", Label: label, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return id
}

func TestResolveRun(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}
	cmd := &cobra.Command{}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	if _, err := a.resolveRun(cmd, "last"); err != errNoRuns {
		t.Fatalf("last with no runs: %v", err)
	}
	if _, err := a.resolveRun(cmd, relativeRunRef+"1"); err != errNoRuns {
		t.Fatalf("-1 with no runs: %v", err)
	}

	first := createLabeledRun(t, st, "")
	bug := createLabeledRun(t, st, "BUG-1234: fix login")
	deploy := createLabeledRun(t, st, "deploy")
	latest := createLabeledRun(t, st, "")

	cases := []struct {
		ref  string
		want int64
	}{
		{"last", latest},
		{relativeRunRef + "1", latest},
		{relativeRunRef + "3", bug},
		{"-2", deploy}, // after "--"
		{fmt.Sprint(first), first},
		{"deploy", deploy},
		{"bug-1234", bug},
		{"1234", bug}, // no run 1234, label matches
	}
	for _, tc := range cases {
		errOut.Reset()
		r, err := a.resolveRun(cmd, tc.ref)
		if err != nil || r.ID != tc.want {
			t.Errorf("resolveRun(%q) = %v, %v; want run %d", displayRef(tc.ref), r, err, tc.want)
		}
		if tc.ref == "1234" && !strings.Contains(errOut.String(), `no run 1234; using run 2 labeled "BUG-1234: fix login"`) {
			t.Errorf("numeric label fallback should say so on stderr, got %q", errOut.String())
		}
		if tc.ref != "1234" && errOut.Len() != 0 {
			t.Errorf("resolveRun(%q) printed %q", displayRef(tc.ref), errOut.String())
		}
	}

	for ref, want := range map[string]string{
		relativeRunRef + "9": "run -9 not found; there are fewer than 9 runs",
		relativeRunRef + "0": `invalid run reference "-0"`,
		"99":                 "run 99 not found",
		"nope":               `no run labeled "nope"`,
		" ":                  "run reference must not be empty",
	} {
		if _, err := a.resolveRun(cmd, ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("resolveRun(%q) err = %v, want %q", displayRef(ref), err, want)
		}
	}
}

func TestResolveSessionLabelFallback(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	bug, _ := st.CreateSession(store.CreateSession{Label: "BUG-1234: Doing something", StartedAt: time.Now()})
	proj, _ := st.CreateSession(store.CreateSession{Label: "PROJ-77 cleanup", StartedAt: time.Now()})
	_, _ = st.CreateSession(store.CreateSession{Label: "OPS-77 rotate", StartedAt: time.Now()})

	cmd := &cobra.Command{}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)

	sess, err := resolveSession(cmd, st, "1234")
	if err != nil || sess.ID != bug {
		t.Fatalf("resolveSession(1234) = %v, %v; want session %d", sess, err, bug)
	}
	if !strings.Contains(errOut.String(), `no session 1234; using session 1 "BUG-1234: Doing something"`) {
		t.Fatalf("fallback should be reported, got %q", errOut.String())
	}

	errOut.Reset()
	for ref, want := range map[string]int64{"BUG-1234": bug, "bug 1234": bug, "proj-77": proj, fmt.Sprint(proj): proj} {
		sess, err := resolveSession(cmd, st, ref)
		if err != nil || sess.ID != want {
			t.Errorf("resolveSession(%q) = %v, %v; want %d", ref, sess, err, want)
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("label and ID matches should not print a hint, got %q", errOut.String())
	}

	if _, err := resolveSession(cmd, st, "77"); err == nil || !strings.Contains(err.Error(), "matches several session labels") {
		t.Fatalf("77 matches two issues and must be ambiguous, got %v", err)
	}
	if _, err := resolveSession(nil, st, "4321"); err == nil || !strings.HasPrefix(err.Error(), "session 4321 not found;") {
		t.Fatalf("missing ID without label match: %v", err)
	}
}

func TestCommandsAcceptRunRefs(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	createLabeledRun(t, st, "")
	target := createLabeledRun(t, st, "deploy")
	createLabeledRun(t, st, "")
	a := &app{st: st}

	out, _, err := runEmpty(t, a.showCmd(), []string{"deploy"}, map[string]string{"json": "true"})
	if err != nil || !strings.Contains(out, fmt.Sprintf(`"id": %d`, target)) {
		t.Fatalf("show deploy: %v\n%s", err, out)
	}
	if _, _, err := runEmpty(t, a.labelCmd(), []string{relativeRunRef + "2", "release"}, nil); err != nil {
		t.Fatalf("label -2: %v", err)
	}
	if r, _ := st.GetRun(target); r.Label != "release" {
		t.Fatalf("label -2 should relabel run %d, got %q", target, r.Label)
	}
	if _, _, err := runEmpty(t, a.noteCmd(), []string{"rolled", "back"}, map[string]string{"run": "release"}); err != nil {
		t.Fatalf("note -r release: %v", err)
	}
	out, _, err = runEmpty(t, a.notesCmd(), nil, map[string]string{"run": relativeRunRef + "2"})
	if err != nil || !strings.Contains(out, "rolled back") {
		t.Fatalf("notes -r -2: %v\n%s", err, out)
	}
}
