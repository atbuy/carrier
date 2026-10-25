package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestSettingsCoverConfig checks that every setting reads the key it names,
// and that every key of the config, except auto_label rules and includes,
// is a setting, so a new key cannot be left out of "carrier config".
func TestSettingsCoverConfig(t *testing.T) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(Default()); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if _, err := toml.Decode(buf.String(), &doc); err != nil {
		t.Fatal(err)
	}
	var keys []string
	var walk func(prefix string, table map[string]any)
	walk = func(prefix string, table map[string]any) {
		for k, v := range table {
			if sub, ok := v.(map[string]any); ok {
				walk(prefix+k+".", sub)
				continue
			}
			if prefix != "" {
				keys = append(keys, prefix+k)
			}
		}
	}
	walk("", doc)
	for _, key := range keys {
		s, ok := LookupSetting(key)
		if !ok {
			t.Errorf("%s is not in Settings", key)
			continue
		}
		var want map[string]any
		if _, err := toml.Decode("v = "+Literal(s.Value(Default())), &want); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		table := doc
		for _, part := range strings.Split(s.Section, ".") {
			table = table[part].(map[string]any)
		}
		if fmt.Sprint(table[s.Key]) != fmt.Sprint(want["v"]) {
			t.Errorf("%s reads %v, the config holds %v", key, want["v"], table[s.Key])
		}
	}
	for _, s := range Settings {
		if !slices.Contains(keys, s.Name()) {
			t.Errorf("setting %s is not a config key", s.Name())
		}
		if s.Summary == "" || s.Title == "" || (s.Kind == KindChoice) != (len(s.Choices) > 0) {
			t.Errorf("setting %s needs a summary, and choices only when it is a choice", s.Name())
		}
	}
}

func TestLiteral(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{true, "true"},
		{int64(20), "20"},
		{"30d", `"30d"`},
		{`AKIA[0-9A-Z]{16}\s`, `'AKIA[0-9A-Z]{16}\s'`},
		{`it's "quoted" \d`, `"it's \"quoted\" \\d"`},
		{[]string{"vim", `\btoken\b`}, `["vim", '\btoken\b']`},
	} {
		got := Literal(tc.value)
		if got != tc.want {
			t.Errorf("Literal(%#v) = %s, want %s", tc.value, got, tc.want)
		}
		var doc map[string]any
		if _, err := toml.Decode("v = "+got, &doc); err != nil {
			t.Errorf("Literal(%#v) is not TOML: %v", tc.value, err)
		}
	}
}

func TestSetValueReplacesMultiLineArray(t *testing.T) {
	content := `[shell]
# programs without output
ignore_commands = [
  "vim",   # editor
  "less]",
] # keep these
ask_outcome = true
`
	got, err := SetValue([]byte(content), "shell", "ignore_commands", `["nvim"]`)
	if err != nil {
		t.Fatal(err)
	}
	want := `[shell]
# programs without output
ignore_commands = ["nvim"] # keep these
ask_outcome = true
`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if removed := string(RemoveValue([]byte(content), "shell", "ignore_commands")); removed != "[shell]\n# programs without output\nask_outcome = true\n" {
		t.Fatalf("RemoveValue:\n%s", removed)
	}
}

func TestSetValueNestedTable(t *testing.T) {
	content := "[ui]\ncolor = \"auto\"\n\n[ui.theme]\naccent = \"#5B8DEF\"\n"
	got, err := SetValue([]byte(content), "ui.theme", "accent", `"#BD93F9"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[ui]\ncolor = \"auto\"\n\n[ui.theme]\naccent = \"#BD93F9\"\n"; string(got) != want {
		t.Fatalf("got:\n%s", got)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setting(t *testing.T, name string) Setting {
	t.Helper()
	s, ok := LookupSetting(name)
	if !ok {
		t.Fatalf("no setting %s", name)
	}
	return s
}

func TestSetCreatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "carrier", "config.toml")
	ask := setting(t, "shell.ask_outcome")
	if src, _ := Source(path, ask); src != "" {
		t.Fatalf("Source without a config = %q", src)
	}
	file, err := Set(path, ask, true)
	if err != nil || file != path {
		t.Fatalf("Set = %q, %v", file, err)
	}
	if got, want := readConfig(t, path), NewFileHeader+"\n[shell]\nask_outcome = true\n"; got != want {
		t.Fatalf("config:\n%s\nwant:\n%s", got, want)
	}
	if src, _ := Source(path, ask); src != path {
		t.Fatalf("Source = %q, want %q", src, path)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSetKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "# my settings\n[notify]\nmin_duration = \"10s\" # long builds\nsuccess = false\n"
	writeConfig(t, path, content)
	if _, err := Set(path, setting(t, "notify.min_duration"), "1m"); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, path); got != strings.Replace(content, `"10s"`, `"1m"`, 1) {
		t.Fatalf("config:\n%s", got)
	}
}

func TestSetRejectsInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[notify]\nmin_duration = \"10s\"\n"
	writeConfig(t, path, content)
	for name, value := range map[string]any{
		"notify.min_duration":   "soon",
		"shell.branch_sessions": "always",
		"storage.max_output_mb": int64(-1),
		"redaction.patterns":    []string{"("},
	} {
		if _, err := Set(path, setting(t, name), value); err == nil {
			t.Errorf("%s = %v was accepted", name, value)
		}
	}
	if got := readConfig(t, path); got != content {
		t.Fatalf("a rejected value changed the config:\n%s", got)
	}
}

func TestSetWritesWhereTheValueTakesEffect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	include := filepath.Join(dir, "work.toml")
	writeConfig(t, path, "includes = [\"work.toml\"]\n\n[ui]\ncolor = \"auto\"\n\n[shell]\nignore_commands = [\"vim\"]\n")
	writeConfig(t, include, "[ui]\ncolor = \"never\"\n\n[shell]\nignore_commands = [\"k9s\"]\n")
	color := setting(t, "ui.color")

	file, err := Set(path, color, "always")
	if err != nil || file != include {
		t.Fatalf("Set = %q, %v; want the include, which overrides the main config", file, err)
	}
	if !strings.Contains(readConfig(t, include), `color = "always"`) {
		t.Fatalf("include:\n%s", readConfig(t, include))
	}
	// Lists are changed in the main config; the include still adds to them.
	if file, err = Set(path, setting(t, "shell.ignore_commands"), []string{"nvim"}); err != nil || file != path {
		t.Fatalf("Set list = %q, %v", file, err)
	}
	// Reset removes the include's value, and the main config's applies.
	if file, err = Reset(path, color); err != nil || file != include {
		t.Fatalf("Reset = %q, %v", file, err)
	}
	if src, _ := Source(path, color); src != path {
		t.Fatalf("Source after reset = %q, want the main config", src)
	}
	cfg, err := load(path, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Color != "auto" || !slices.Equal(cfg.Shell.IgnoreCommands, []string{"nvim", "k9s"}) {
		t.Fatalf("config = %+v %+v", cfg.UI, cfg.Shell)
	}
	if file, err = Reset(path, setting(t, "hints.similar_failures")); err != nil || file != "" {
		t.Fatalf("Reset of an unset setting = %q, %v", file, err)
	}
}

func TestSetWritesThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "carrier.toml")
	writeConfig(t, real, "[ui]\ncolor = \"auto\"\n")
	path := filepath.Join(dir, "config.toml")
	if err := os.Symlink(real, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Set(path, setting(t, "ui.color"), "never"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced")
	}
	if !strings.Contains(readConfig(t, real), `color = "never"`) {
		t.Fatalf("linked file:\n%s", readConfig(t, real))
	}
}

func TestRemoveValueDropsEmptyTable(t *testing.T) {
	for _, tc := range []struct{ name, content, section, key, want string }{
		{
			"last table", "# mine\n\n[shell]\nask_outcome = true\n", "shell", "ask_outcome",
			"# mine\n",
		},
		{
			"between tables", "[notify]\nsuccess = false\n\n[shell]\nask_outcome = true\n\n[ui]\ncolor = \"never\"\n", "shell", "ask_outcome",
			"[notify]\nsuccess = false\n\n[ui]\ncolor = \"never\"\n",
		},
		{
			"first table", "[shell]\nask_outcome = true\n\n[ui]\ncolor = \"never\"\n", "shell", "ask_outcome",
			"[ui]\ncolor = \"never\"\n",
		},
		{
			"only table", "[shell]\nask_outcome = true\n", "shell", "ask_outcome",
			"",
		},
		{
			"other keys left", "[shell]\nask_outcome = true\nprogram = \"zsh\"\n", "shell", "ask_outcome",
			"[shell]\nprogram = \"zsh\"\n",
		},
		{
			"comment left", "[shell]\n# for work\nask_outcome = true\n", "shell", "ask_outcome",
			"[shell]\n# for work\n",
		},
		{
			"nested table", "[ui]\ncolor = \"never\"\n\n[ui.theme]\naccent = \"#BD93F9\"\n", "ui.theme", "accent",
			"[ui]\ncolor = \"never\"\n",
		},
	} {
		got := string(RemoveValue([]byte(tc.content), tc.section, tc.key))
		if got != tc.want {
			t.Errorf("%s:\ngot:\n%q\nwant:\n%q", tc.name, got, tc.want)
		}
		var doc map[string]any
		if _, err := toml.Decode(got, &doc); err != nil {
			t.Errorf("%s: result is not TOML: %v", tc.name, err)
		}
	}
}
