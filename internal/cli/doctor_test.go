package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
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

func TestDoctorCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg}

	cmd := a.doctorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("doctorCmd: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected non-empty doctor output")
	}
}

func TestRunDoctor(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dir

	a := &app{st: st, cfg: cfg}
	var out bytes.Buffer
	if err := a.runDoctor(&out); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	for _, want := range []string{"version", "data dir", "sqlite db", "migration"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("runDoctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCheckOk(t *testing.T) {
	var out bytes.Buffer
	c := outputColors(&out)
	check(&out, c, "mycheck", true, "all good", nil)
	if !strings.Contains(out.String(), "mycheck") {
		t.Fatalf("check output missing name:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("check output missing 'ok':\n%s", out.String())
	}
}

func TestCheckWarn(t *testing.T) {
	var out bytes.Buffer
	c := outputColors(&out)
	check(&out, c, "badcheck", false, "some warning", nil)
	if !strings.Contains(out.String(), "warn") {
		t.Fatalf("check output should show 'warn':\n%s", out.String())
	}
}

func TestCheckError(t *testing.T) {
	var out bytes.Buffer
	c := outputColors(&out)
	check(&out, c, "errcheck", false, "detail", os.ErrNotExist)
	if !strings.Contains(out.String(), os.ErrNotExist.Error()) {
		t.Fatalf("check output should show error detail:\n%s", out.String())
	}
}

func TestShellProgramNotSet(t *testing.T) {
	t.Setenv("SHELL", "")
	got := shellProgram("")
	if got != "(not set)" {
		t.Fatalf("shellProgram with no SHELL = %q", got)
	}
}

func TestFormatBytesMiB(t *testing.T) {
	// 2MiB
	if got := formatBytes(2 * 1024 * 1024); got != "2.0 MiB" {
		t.Fatalf("formatBytes 2MiB = %q", got)
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
