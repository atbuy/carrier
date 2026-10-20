package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetValue(t *testing.T) {
	base := `# my carrier config
[storage]
data_dir = "~/data" # where runs go

[notify]
min_duration = "5s"  # quick builds
success = false

[[auto_label]]
label = "tests"
cmd = 'go test.*'
`
	cases := []struct {
		name, section, key, value, want string
	}{
		{
			"replace keeps comment and spacing", "notify", "min_duration", `"30s"`,
			strings.Replace(base, `min_duration = "5s"  # quick builds`, `min_duration = "30s"  # quick builds`, 1),
		},
		{
			"replace without comment", "notify", "success", "true",
			strings.Replace(base, "success = false", "success = true", 1),
		},
		{
			"insert into existing section", "storage", "max_output_mb", "50",
			strings.Replace(base, "data_dir = \"~/data\" # where runs go\n", "data_dir = \"~/data\" # where runs go\nmax_output_mb = 50\n", 1),
		},
		{
			"new section at end", "hints", "similar_failures", "true",
			base + "\n[hints]\nsimilar_failures = true\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SetValue([]byte(base), tc.section, tc.key, tc.value)
			if err != nil {
				t.Fatalf("SetValue: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestSetValueEdgeCases(t *testing.T) {
	got, err := SetValue(nil, "hints", "similar_failures", "true")
	if err != nil || string(got) != "[hints]\nsimilar_failures = true\n" {
		t.Fatalf("empty file: %q, %v", got, err)
	}
	got, err = SetValue([]byte("[shell]\nprogram = \"zsh\""), "shell", "auto_session_retention", `"7d"`)
	if err != nil || string(got) != "[shell]\nprogram = \"zsh\"\nauto_session_retention = \"7d\"" {
		t.Fatalf("no trailing newline: %q, %v", got, err)
	}
	// A key set with a dotted name elsewhere would be duplicated; the result
	// fails validation instead of being written.
	if _, err := SetValue([]byte("hints.similar_failures = false\n"), "hints", "similar_failures", "true"); err == nil {
		t.Fatal("expected an error rather than a duplicate key")
	}
	if _, err := SetValue([]byte("[hints]\n"), "hints", "similar_failures", "not-toml"); err == nil {
		t.Fatal("expected an error for an invalid value")
	}
	got, err = SetValue([]byte("[ui]\ncolor = \"auto\" # pick # wisely\n"), "ui", "color", `"never"`)
	if err != nil || string(got) != "[ui]\ncolor = \"never\" # pick # wisely\n" {
		t.Fatalf("comment after string: %q, %v", got, err)
	}
	if trailingComment(`"a # b"`) != "" || trailingComment(`true # it's on`) != " # it's on" || trailingComment(`1#x`) != " #x" {
		t.Fatal("trailingComment should ignore '#' inside strings and keep quotes in comments")
	}
}

func TestDefinedIn(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config.toml", "includes = [\"conf.d/*.toml\"]\n[notify]\nmin_duration = \"5s\"\n")
	_ = os.MkdirAll(filepath.Join(dir, "conf.d"), 0o755)
	write("conf.d/a.toml", "[hints]\nsimilar_failures = false\n")
	write("conf.d/b.toml", "[hints]\nsimilar_failures = true\n")

	for _, tc := range []struct{ section, key, want string }{
		{"notify", "min_duration", mainPath},
		{"hints", "similar_failures", filepath.Join(dir, "conf.d", "b.toml")},
		{"shell", "program", mainPath},
	} {
		got, err := DefinedIn(mainPath, tc.section, tc.key)
		if err != nil || got != tc.want {
			t.Errorf("DefinedIn(%s.%s) = %q, %v; want %q", tc.section, tc.key, got, err, tc.want)
		}
	}
	write("broken.toml", "[notify\n")
	if _, err := DefinedIn(filepath.Join(dir, "broken.toml"), "notify", "x"); err == nil {
		t.Fatal("expected parse error")
	}
}
