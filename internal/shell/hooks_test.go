package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHookDirZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/zsh")
	if err != nil {
		t.Fatalf("WriteHookDir zsh: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	contents, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read zsh hook: %v", err)
	}
	text := string(contents)
	for _, want := range []string{"add-zsh-hook preexec", "internal begin", "internal end", "/tmp/state.json", "CARRIER_HOOK_ACTIVE", "unsetopt warn_create_global", "_carrier_disable_prompt_warnings", "precmd_functions=(_carrier_disable_prompt_warnings"} {
		if !strings.Contains(text, want) {
			t.Fatalf("zsh hook missing %q\n%s", want, text)
		}
	}
}

func TestWriteHookDirUnsupportedShellFallsBackToZsh(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/fish")
	if err != nil {
		t.Fatalf("WriteHookDir fish: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Unsupported shell → falls back to .zshrc
	if _, err := os.Stat(filepath.Join(dir, ".zshrc")); err != nil {
		t.Fatalf("fallback .zshrc missing: %v", err)
	}
}

func TestWriteHookDirBash(t *testing.T) {
	dir, err := WriteHookDir("/bin/carrier", "/tmp/state.json", "/bin/bash")
	if err != nil {
		t.Fatalf("WriteHookDir bash: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	contents, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	if err != nil {
		t.Fatalf("read bash hook: %v", err)
	}
	text := string(contents)
	for _, want := range []string{"PROMPT_COMMAND", "_carrier_original_prompt_command", "trap _carrier_begin DEBUG", "internal begin", "internal end", "CARRIER_HOOK_ACTIVE"} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash hook missing %q\n%s", want, text)
		}
	}
}
