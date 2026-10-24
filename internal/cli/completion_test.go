package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

// complete runs cobra's completion request for args, as the shell scripts do
// on Tab, and returns the offered values, their descriptions, and the
// directive.
func complete(t *testing.T, a *app, args ...string) ([]string, []string, cobra.ShellCompDirective) {
	t.Helper()
	root := newRootCmd(a)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{cobra.ShellCompRequestCmd}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("complete %q: %v", args, err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var directive int
	if _, err := fmt.Sscanf(lines[len(lines)-1], ":%d", &directive); err != nil {
		t.Fatalf("complete %q: no directive in %q", args, out.String())
	}
	var values, descs []string
	for _, line := range lines[:len(lines)-1] {
		value, desc, _ := strings.Cut(line, "\t")
		values = append(values, value)
		descs = append(descs, desc)
	}
	return values, descs, cobra.ShellCompDirective(directive)
}

// completionApp records sessions and runs for the completion tests.
func completionApp(t *testing.T) (*app, map[string]int64) {
	t.Helper()
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	t.Cleanup(func() { _ = st.Close() })
	a := &app{st: st}
	ids := map[string]int64{}
	started := time.Now().Add(-150 * time.Minute)
	for _, s := range []struct {
		name, label string
		kept        bool
	}{{"bug", "BUG-1234: fix login", false}, {"kept", "api refactor", true}, {"other", "", false}} {
		id, err := st.CreateSession(store.CreateSession{Label: s.label, StartedAt: started})
		if err != nil {
			t.Fatal(err)
		}
		if s.kept {
			if err := st.SetSessionKept(id, true); err != nil {
				t.Fatal(err)
			}
		}
		ids[s.name] = id
	}
	for i, r := range []struct{ name, cmd, label, status, branch string }{
		{"failed", "go test ./...", "", store.StatusFailed, "main"},
		{"deploy", "kubectl apply -f k8s", "deploy prod", store.StatusSuccess, "BUG-1234-fix"},
		{"running", "npm run dev", "", store.StatusRunning, "BUG-1234-fix"},
	} {
		sess := ids["bug"]
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeShell, Command: r.cmd, ArgvJSON: "[]", CWD: "/home/dev/api",
			Label: r.label, StartedAt: started.Add(time.Duration(i) * time.Minute), GitBranch: r.branch, SessionID: &sess,
		})
		if err != nil {
			t.Fatal(err)
		}
		if r.status != store.StatusRunning {
			if err := st.FinishRun(id, r.status, 1, started.Add(time.Duration(i)*time.Minute+time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		ids[r.name] = id
	}
	return a, ids
}

func ids(values ...int64) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprint(v)
	}
	return out
}

func TestCompleteRuns(t *testing.T) {
	a, id := completionApp(t)

	values, descs, directive := complete(t, a, "show", "")
	if want := ids(id["running"], id["deploy"], id["failed"]); !slices.Equal(values, want) {
		t.Fatalf("show: got %q, want the newest runs first %q", values, want)
	}
	if want := "success · 2h ago · kubectl apply -f k8s · deploy prod"; descs[1] != want {
		t.Errorf("description = %q, want %q", descs[1], want)
	}
	if directive != noFiles {
		t.Errorf("directive = %v, want no files, in order", directive)
	}

	// Letters complete run labels, without repeating the label.
	values, descs, _ = complete(t, a, "rerun", "dep")
	if !slices.Equal(values, []string{"deploy prod"}) || strings.Contains(descs[0], "deploy prod") {
		t.Errorf("rerun dep: got %q %q", values, descs)
	}

	if values, _, _ = complete(t, a, "tail", ""); !slices.Equal(values, ids(id["running"])) {
		t.Errorf("tail: got %q, want only the running run", values)
	}
	// diff completes two runs; the second leaves out the first.
	if values, _, _ = complete(t, a, "diff", fmt.Sprint(id["failed"]), ""); !slices.Equal(values, ids(id["running"], id["deploy"])) {
		t.Errorf("diff second run: got %q", values)
	}
	if values, _, _ = complete(t, a, "diff", "1", "2", ""); len(values) != 0 {
		t.Errorf("diff third argument: got %q, want nothing", values)
	}
	// Only the first argument of label is a run; the rest is text.
	if values, _, _ = complete(t, a, "label", fmt.Sprint(id["failed"]), ""); len(values) != 0 {
		t.Errorf("label text: got %q, want nothing", values)
	}
	if values, _, _ = complete(t, a, "note", "--run", ""); len(values) != 3 {
		t.Errorf("note --run: got %q, want runs", values)
	}
}

func TestCompleteSessions(t *testing.T) {
	a, id := completionApp(t)

	values, descs, _ := complete(t, a, "attach", "")
	if want := ids(id["other"], id["kept"], id["bug"]); !slices.Equal(values, want) {
		t.Fatalf("attach: got %q, want %q", values, want)
	}
	if want := "BUG-1234: fix login · 3 runs · started 2h ago"; descs[2] != want {
		t.Errorf("description = %q, want %q", descs[2], want)
	}
	if want := "api refactor · 0 runs · started 2h ago · kept"; descs[1] != want {
		t.Errorf("description = %q, want %q", descs[1], want)
	}

	// attach leaves out the session this shell records into.
	t.Setenv("CARRIER_SESSION_ID", fmt.Sprint(id["other"]))
	if values, _, _ = complete(t, a, "attach", ""); slices.Contains(values, fmt.Sprint(id["other"])) {
		t.Errorf("attach offered the current session: %q", values)
	}
	t.Setenv("CARRIER_SESSION_ID", "")

	if values, _, _ = complete(t, a, "a", "BUG"); !slices.Equal(values, []string{"BUG-1234: fix login"}) {
		t.Errorf("attach BUG: got %q", values)
	}
	if values, _, _ = complete(t, a, "session", "keep", ""); slices.Contains(values, fmt.Sprint(id["kept"])) {
		t.Errorf("session keep offered a kept session: %q", values)
	}
	if values, _, _ = complete(t, a, "session", "unkeep", ""); !slices.Equal(values, ids(id["kept"])) {
		t.Errorf("session unkeep: got %q, want only the kept session", values)
	}
	// "session label" takes text after an optional ID, so letters are text.
	if values, _, _ = complete(t, a, "session", "label", "fix"); len(values) != 0 {
		t.Errorf("session label text: got %q, want nothing", values)
	}
	if values, _, _ = complete(t, a, "history", "--session", "api"); !slices.Equal(values, []string{"api refactor"}) {
		t.Errorf("history --session: got %q", values)
	}
}

func TestCompleteFlagValues(t *testing.T) {
	a, _ := completionApp(t)
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"history", "--status", ""}, []string{"success", "failed", "running", "killed"}},
		{[]string{"hs", "--branch", ""}, []string{"BUG-1234-fix", "main"}},
		{[]string{"history", "--command", "go"}, []string{"go test ./..."}},
		{[]string{"history", "--cwd", ""}, []string{"/home/dev/api"}},
		{[]string{"export", "--format", ""}, []string{"markdown", "json", "csv"}},
		{[]string{"session", "export", "--format", ""}, []string{"markdown", "json"}},
		{[]string{"tail", "--stream", ""}, []string{"both", "stdout", "stderr", "terminal"}},
		{[]string{"setup", "--shell", ""}, []string{"zsh", "bash", "fish"}},
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish", "powershell"}},
	} {
		if values, _, _ := complete(t, a, tc.args...); !slices.Equal(values, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.args, values, tc.want)
		}
	}
	values, _, _ := complete(t, a, "clean", "--older-than", "")
	if !slices.Contains(values, "30d") {
		t.Errorf("clean --older-than: got %q, want durations", values)
	}
	values, descs, _ := complete(t, a, "self", "update", "--version", "")
	if !slices.Contains(values, "1.0.0") || descs[0] == "" {
		t.Errorf("self update --version: got %q %q, want released versions with dates", values, descs)
	}
}

func TestCompleteOffersNoFiles(t *testing.T) {
	a, _ := completionApp(t)
	for _, args := range [][]string{
		{"status", ""},
		{"last", ""},
		{"note", ""},
		{"search", ""},
		{"run", "git", ""},
		{"show", "1", ""},
		{"history", "--limit", ""},
		{"rerun", "--label", ""},
		{"session", "end", "--note", ""},
	} {
		values, _, directive := complete(t, a, args...)
		if len(values) != 0 || directive&cobra.ShellCompDirectiveNoFileComp == 0 {
			t.Errorf("%q: got %q with directive %v, want nothing and no files", args, values, directive)
		}
	}
	// Flags that take a path keep file completion.
	for _, args := range [][]string{{"session", "export", "--output-file", ""}, {"shell", "--program", ""}} {
		if _, _, directive := complete(t, a, args...); directive&cobra.ShellCompDirectiveNoFileComp != 0 {
			t.Errorf("%q: want file completion, got directive %v", args, directive)
		}
	}
}

// TestCompletionWithoutStore checks that completion never creates the data
// directory and offers nothing, without an error, when there is no store.
func TestCompletionWithoutStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	values, _, directive := complete(t, &app{}, "show", "")
	if len(values) != 0 || directive != noFiles {
		t.Fatalf("got %q with directive %v", values, directive)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("completion wrote to %s: %v", home, entries)
	}
}

// TestCompletionSpecsMatchCommands catches a renamed command or flag that
// would silently lose its completion.
func TestCompletionSpecsMatchCommands(t *testing.T) {
	a := &app{}
	root := newRootCmd(a)
	find := func(path string) *cobra.Command {
		t.Helper()
		cmd, _, err := root.Find(strings.Fields(path)[1:])
		if err != nil || cmd.CommandPath() != path {
			t.Fatalf("no command %q", path)
		}
		return cmd
	}
	specs := a.completionSpecs()
	for path := range specs.args {
		find(path)
	}
	for path, flags := range specs.flags {
		cmd := find(path)
		for name := range flags {
			if cmd.Flags().Lookup(name) == nil {
				t.Errorf("%s has no --%s flag", path, name)
			}
		}
	}
	for flag := range specs.fileFlags {
		path, name, _ := strings.Cut(flag, " --")
		if find(path).Flags().Lookup(name) == nil {
			t.Errorf("%s has no --%s flag", path, name)
		}
	}
}

func TestShortText(t *testing.T) {
	if got := shortText("make\n  build   all", 50); got != "make build all" {
		t.Errorf("got %q", got)
	}
	if got := shortText("abcdefghij", 5); got != "abcd…" {
		t.Errorf("got %q", got)
	}
}
