package cli

import (
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
