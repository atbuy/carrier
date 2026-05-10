package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorWritableChecks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	if !dirWritable(dir) {
		t.Fatalf("expected temp data dir to be writable")
	}
	if !fileParentWritable(filepath.Join(dir, "carrier.db")) {
		t.Fatalf("expected sqlite parent dir to be writable")
	}
}

func TestDoctorCommandAvailable(t *testing.T) {
	if commandAvailable("definitely-not-a-carrier-test-command") {
		t.Fatalf("unexpected command availability")
	}
}

func TestDoctorShellProgramAndSupport(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	if got := shellProgram(""); got != "/bin/zsh" {
		t.Fatalf("shellProgram env fallback = %q", got)
	}
	if got := shellProgram("/usr/bin/bash"); got != "/usr/bin/bash" {
		t.Fatalf("shellProgram configured = %q", got)
	}
	if !shellSupported("/usr/bin/bash") {
		t.Fatalf("bash should be supported")
	}
	if !shellSupported("/bin/zsh") {
		t.Fatalf("zsh should be supported")
	}
	if shellSupported("/bin/fish") {
		t.Fatalf("fish should not be supported")
	}
}

func TestDoctorDirSizeAndFormatBytes(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.log"), "12345")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "nested", "b.log"), "123")

	if got := dirSize(dir); got != 8 {
		t.Fatalf("dirSize = %d", got)
	}
	if got := formatBytes(512); got != "512 B" {
		t.Fatalf("formatBytes bytes = %q", got)
	}
	if got := formatBytes(2048); got != "2.0 KiB" {
		t.Fatalf("formatBytes kib = %q", got)
	}
}
