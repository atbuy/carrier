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
	for _, want := range []string{"add-zsh-hook preexec", "internal begin", "internal end", "/tmp/state.json"} {
		if !strings.Contains(text, want) {
			t.Fatalf("zsh hook missing %q\n%s", want, text)
		}
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
	for _, want := range []string{"PROMPT_COMMAND", "trap", "internal begin", "internal end"} {
		if !strings.Contains(text, want) {
			t.Fatalf("bash hook missing %q\n%s", want, text)
		}
	}
}
