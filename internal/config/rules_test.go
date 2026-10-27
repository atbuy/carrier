package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rulesFile = `# carrier configuration

[notify]
shell = true

# Tests in any project.
[[auto_label]]
label = "tests" # shown in history
cmd = 'go test.*'

[[auto_label]]
label = "frontend"
dir = '/home/dev/work/frontend'

[shell]
auto_sessions = true
`

func writeRules(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAddAutoLabelGoesAfterTheRules(t *testing.T) {
	path := writeRules(t, rulesFile)
	if err := AddAutoLabel(path, AutoLabel{Label: "feature ${branch}", GitBranch: `feat/(?P<branch>.+)`}); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rulesFile, "dir = '/home/dev/work/frontend'\n", "dir = '/home/dev/work/frontend'\n\n[[auto_label]]\nlabel = \"feature ${branch}\"\ngit_branch = \"feat/(?P<branch>.+)\"\n", 1)
	if got := readFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAddAutoLabelToNewOrEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := AddAutoLabel(path, AutoLabel{Label: "build", Cmd: `make\b`}); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), NewFileHeader+"\n[[auto_label]]\nlabel = \"build\"\ncmd = 'make\\b'\n"; got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	rules, err := MainAutoLabels(path)
	if err != nil || len(rules) != 1 || rules[0].Cmd != `make\b` {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
}

func TestSetAutoLabelKeepsCommentsAndOrder(t *testing.T) {
	path := writeRules(t, rulesFile)
	if err := SetAutoLabel(path, 0, AutoLabel{Label: "unit tests", Dir: "/home/dev/src"}); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rulesFile, "label = \"tests\" # shown in history\ncmd = 'go test.*'\n", "label = \"unit tests\" # shown in history\ndir = \"/home/dev/src\"\n", 1)
	if got := readFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetAutoLabelKeepsUnchangedKeys(t *testing.T) {
	path := writeRules(t, rulesFile)
	if err := SetAutoLabel(path, 0, AutoLabel{Label: "unit tests", Cmd: "go test.*"}); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rulesFile, `label = "tests" # shown in history`, `label = "unit tests" # shown in history`, 1)
	if got := readFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemoveAutoLabel(t *testing.T) {
	path := writeRules(t, rulesFile)
	if err := RemoveAutoLabel(path, 0); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rulesFile, "# Tests in any project.\n[[auto_label]]\nlabel = \"tests\" # shown in history\ncmd = 'go test.*'\n\n", "", 1)
	if got := readFile(t, path); got != want {
		t.Fatalf("first rule:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if err := RemoveAutoLabel(path, 0); err != nil {
		t.Fatal(err)
	}
	want = "# carrier configuration\n\n[notify]\nshell = true\n\n[shell]\nauto_sessions = true\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("last rule:\ngot:\n%s\nwant:\n%s", got, want)
	}
	// A rule at the end of the file takes the blank line before it along.
	path = writeRules(t, "[notify]\nshell = true\n\n[[auto_label]]\nlabel = \"x\"\n")
	if err := RemoveAutoLabel(path, 0); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "[notify]\nshell = true\n" {
		t.Fatalf("rule at the end: %q", got)
	}
}

func TestMoveAutoLabel(t *testing.T) {
	path := writeRules(t, rulesFile)
	if err := MoveAutoLabel(path, 1, 0); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(rulesFile, "# Tests in any project.\n[[auto_label]]\nlabel = \"tests\" # shown in history\ncmd = 'go test.*'\n\n[[auto_label]]\nlabel = \"frontend\"\ndir = '/home/dev/work/frontend'\n",
		"[[auto_label]]\nlabel = \"frontend\"\ndir = '/home/dev/work/frontend'\n\n# Tests in any project.\n[[auto_label]]\nlabel = \"tests\" # shown in history\ncmd = 'go test.*'\n", 1)
	if got := readFile(t, path); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	rules, _ := MainAutoLabels(path)
	if rules[0].Label != "frontend" || rules[1].Label != "tests" {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestAutoLabelEditsAreChecked(t *testing.T) {
	path := writeRules(t, rulesFile)
	for _, bad := range []AutoLabel{{Label: " ", Cmd: "x"}, {Label: "x", Cmd: "("}} {
		if err := AddAutoLabel(path, bad); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
	if err := SetAutoLabel(path, 5, AutoLabel{Label: "x"}); err == nil {
		t.Error("rule 6 does not exist")
	}
	if got := readFile(t, path); got != rulesFile {
		t.Fatalf("refused edits changed the file:\n%s", got)
	}
	// Rules written as an inline array are left to the file.
	inline := writeRules(t, "auto_label = [{label = \"x\", cmd = \"y\"}]\n")
	if err := AddAutoLabel(inline, AutoLabel{Label: "z"}); err == nil || !strings.Contains(err.Error(), "edit them in the file") {
		t.Fatalf("inline rules: err = %v", err)
	}
}

func TestMatchAutoLabel(t *testing.T) {
	cfg := Default()
	cfg.AutoLabels = []AutoLabel{{Label: "tests", Cmd: "go test"}, {Label: "feature ${b}", GitBranch: "feat/(?P<b>.+)"}}
	if err := cfg.CompileAutoLabels(); err != nil {
		t.Fatal(err)
	}
	if label, i := cfg.MatchAutoLabel("make", "/src", "feat/login"); label != "feature login" || i != 1 {
		t.Errorf("got %q, %d", label, i)
	}
	if label, i := cfg.MatchAutoLabel("make", "/src", "main"); label != "" || i != -1 {
		t.Errorf("got %q, %d", label, i)
	}
}
