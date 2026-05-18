package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHookDirZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/zsh", 3, "my-label")
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
		"session #3", "my-label", `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("zsh hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirFish(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/fish", 7, "work")
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
		"CARRIER_HOOK_ACTIVE", "session #7", "work", `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("fish hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirUnsupportedShellFallsBackToZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/unknown", 1, "")
	if err != nil {
		t.Fatalf("WriteHookDir unknown: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if _, err := os.Stat(filepath.Join(dir, ".zshrc")); err != nil {
		t.Fatalf("fallback .zshrc missing: %v", err)
	}
}

func TestWriteHookDirBash(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/bash", 5, "")
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
		"session #5", `\033]0;`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash hook missing %q\n%s", want, text)
		}
	}
}
