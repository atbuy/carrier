package shell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateFileReadWrite(t *testing.T) {
	sf := &StateFile{Path: filepath.Join(t.TempDir(), "state.json")}

	if got := sf.Read(); got.CurrentID != 0 || got.CurrentLog != "" {
		t.Fatalf("missing state should read zero value: %#v", got)
	}

	want := State{CurrentID: 42, CurrentLog: "/tmp/run.log"}
	if err := sf.Write(want); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if got := sf.Read(); got != want {
		t.Fatalf("state mismatch: got %#v want %#v", got, want)
	}
}

func TestWarnShellAlphaOnceWritesMarker(t *testing.T) {
	dir := t.TempDir()

	warnShellAlphaOnce(dir)
	if _, err := os.Stat(filepath.Join(dir, "shell-alpha-warning")); err != nil {
		t.Fatalf("warning marker missing: %v", err)
	}
	warnShellAlphaOnce(dir)
}

func TestShellArgsUsesBashRcFile(t *testing.T) {
	dir := t.TempDir()

	got := shellArgs("/bin/bash", dir)
	want := []string{"--rcfile", filepath.Join(dir, ".bashrc"), "-i"}
	if len(got) != len(want) {
		t.Fatalf("shell args length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shell arg %d = %q, want %q", i, got[i], want[i])
		}
	}
	if got := shellArgs("/bin/zsh", dir); len(got) != 1 || got[0] != "-i" {
		t.Fatalf("zsh args = %#v", got)
	}
}
