package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHookDirZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/zsh", 3, "my-label", "#FF00AA", true)
	if err != nil {
		t.Fatalf("WriteHookDir zsh: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	contents, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read zsh hook: %v", err)
	}
	text := string(contents)
	for _, want := range []string{
		"add-zsh-hook preexec", "internal begin", "internal end", "/tmp/state.json",
		"CARRIER_HOOK_ACTIVE", "unsetopt warn_create_global", "zle -N zle-line-init _carrier_zle_init",
		"_carrier_session_id=3", "_carrier_session_label='my-label'", "_carrier_session_file='/tmp/state.json.session'",
		"%B%F{#FF00AA}carrier #${_carrier_session_id}%f%b", " %F{#FF00AA}${_carrier_session_label//", `\033[38;2;255;0;170;1mcarrier #3`, `\033[38;2;255;0;170mmy-label`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("zsh hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirFish(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/fish", 7, "work", "#00FF66", true)
	if err != nil {
		t.Fatalf("WriteHookDir fish: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	contents, err := os.ReadFile(filepath.Join(dir, "carrier.fish"))
	if err != nil {
		t.Fatalf("read fish hook: %v", err)
	}
	text := string(contents)
	for _, want := range []string{
		"fish_preexec", "fish_postexec", "internal begin", "internal end",
		"CARRIER_HOOK_ACTIVE", "set -g _carrier_session_id 7", "set -g _carrier_session_label 'work'", "set -g _carrier_session_file '/tmp/state.json.session'",
		"set_color --bold 00FF66", "set_color 00FF66", `echo -n "carrier #$_carrier_session_id"`, `\033[38;2;0;255;102;1mcarrier #7`, `\033[38;2;0;255;102mwork`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("fish hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirUnsupportedShellFallsBackToZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/unknown", 1, "", "#FF00AA", true)
	if err != nil {
		t.Fatalf("WriteHookDir unknown: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if _, err := os.Stat(filepath.Join(dir, ".zshrc")); err != nil {
		t.Fatalf("fallback .zshrc missing: %v", err)
	}
}

func TestWriteHookDirBash(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/bash", 5, "build", "#FF00AA", true)
	if err != nil {
		t.Fatalf("WriteHookDir bash: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	contents, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	if err != nil {
		t.Fatalf("read bash hook: %v", err)
	}
	text := string(contents)
	for _, want := range []string{
		"PROMPT_COMMAND", "_carrier_original_prompt_command", "trap _carrier_begin DEBUG",
		"internal begin", "internal end", "CARRIER_HOOK_ACTIVE", "_carrier_at_prompt", `PS0="${PS0:-}"'${_carrier_read=}$(_carrier_ps0)'`,
		"_carrier_session_id=5", "_carrier_session_label='build'", "_carrier_session_file='/tmp/state.json.session'",
		`\033[38;2;255;0;170;1m\]carrier #${_carrier_session_id}`, `\033[38;2;255;0;170m\]${_carrier_session_label}`, `\033[38;2;255;0;170mbuild`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash hook missing %q\n%s", want, text)
		}
	}
	// The DEBUG trap must be installed after the banner and title commands so
	// they are not recorded as runs.
	if strings.LastIndex(text, "trap _carrier_begin DEBUG") < strings.LastIndex(text, `\033]0;`) {
		t.Fatalf("DEBUG trap must be installed last\n%s", text)
	}
}

func TestShellLabelColorFormats(t *testing.T) {
	cases := []struct {
		color string
		bold  bool
		want  string
	}{
		{"#01020A", false, `\033[38;2;1;2;10m`},
		{"#01020A", true, `\033[38;2;1;2;10;1m`},
		{"110", false, `\033[38;5;110m`},
		{"110", true, `\033[38;5;110;1m`},
		{"red", false, `\033[31m`},
		{"red", true, `\033[31;1m`},
		{"bogus", false, ``},
		{"", false, ``},
	}
	for _, tc := range cases {
		if got := shellANSIColor(tc.color, tc.bold); got != tc.want {
			t.Fatalf("shellANSIColor(%q, bold=%v) = %q, want %q", tc.color, tc.bold, got, tc.want)
		}
	}
	if got := zshColor(""); got != "" {
		t.Fatalf("zshColor empty = %q", got)
	}
	if got := fishColor("#01020A"); got != "01020A" {
		t.Fatalf("fishColor hex = %q", got)
	}
}

func TestWriteHookDirWithoutBanner(t *testing.T) {
	for _, program := range []string{"/bin/zsh", "/bin/bash", "/bin/fish"} {
		dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", program, 4, "quiet", "#FF00AA", false)
		if err != nil {
			t.Fatalf("WriteHookDir %s: %v", program, err)
		}
		entries, _ := os.ReadDir(dir)
		contents, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
		_ = os.RemoveAll(dir)
		if strings.Contains(string(contents), shellBanner(4, "quiet", "#FF00AA")) {
			t.Fatalf("%s hooks should not print the banner when quiet", program)
		}
		if !strings.Contains(string(contents), "internal begin") {
			t.Fatalf("%s hooks missing tracking", program)
		}
	}
}

func TestHooksAskOutcomeOnExit(t *testing.T) {
	for program, want := range map[string]string{
		"/bin/zsh":  "add-zsh-hook zshexit _carrier_zshexit",
		"/bin/bash": `[[ -z "$(trap -p EXIT)" ]] && trap _carrier_exit EXIT`,
		"/bin/fish": "function _carrier_exit --on-event fish_exit",
	} {
		dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", program, 1, "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		entries, _ := os.ReadDir(dir)
		contents, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
		_ = os.RemoveAll(dir)
		text := string(contents)
		if !strings.Contains(text, want) || !strings.Contains(text, "internal outcome") || !strings.Contains(text, "CARRIER_ASK_OUTCOME") {
			t.Errorf("%s hooks should ask for the outcome on exit when CARRIER_ASK_OUTCOME is set:\n%s", program, text)
		}
	}
}
