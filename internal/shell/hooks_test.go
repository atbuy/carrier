package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHookDirZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/zsh", 3, "my-label", "#FF00AA")
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
		"my-label", "%B%F{#FF00AA}carrier #3%f%b", " %F{#FF00AA}my-label%f", `\033[38;2;255;0;170;1mcarrier #3`, `\033[38;2;255;0;170mmy-label`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("zsh hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirFish(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/fish", 7, "work", "#00FF66")
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
		"CARRIER_HOOK_ACTIVE", "work", "set_color --bold 00FF66", "set_color 00FF66", `\033[38;2;0;255;102;1mcarrier #7`, `\033[38;2;0;255;102mwork`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("fish hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirUnsupportedShellFallsBackToZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/unknown", 1, "", "#FF00AA")
	if err != nil {
		t.Fatalf("WriteHookDir unknown: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if _, err := os.Stat(filepath.Join(dir, ".zshrc")); err != nil {
		t.Fatalf("fallback .zshrc missing: %v", err)
	}
}

func TestWriteHookDirBash(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/bash", 5, "build", "#FF00AA")
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
		"internal begin", "internal end", "CARRIER_HOOK_ACTIVE",
		"build", `\033[38;2;255;0;170;1m`, `\033[38;2;255;0;170mbuild`, `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash hook missing %q\n%s", want, text)
		}
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
