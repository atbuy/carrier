package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecuteVersionReturnsZero(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"carrier", "version"}
	t.Cleanup(func() { os.Args = oldArgs })

	if code := Execute(); code != 0 {
		t.Fatalf("Execute(version) = %d, want 0", code)
	}
}

func TestExecuteUnknownCommandReturnsOne(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"carrier", "does-not-exist"}
	t.Cleanup(func() { os.Args = oldArgs })

	if code := Execute(); code != 1 {
		t.Fatalf("Execute(unknown) = %d, want 1", code)
	}
}

func TestStaleCheckDue(t *testing.T) {
	dir := t.TempDir()

	// no file → always due
	if !staleCheckDue(dir, 5*time.Minute) {
		t.Fatal("expected due when timestamp file absent")
	}

	// write timestamp, check immediately → not due
	touchStaleCheck(dir)
	if staleCheckDue(dir, 5*time.Minute) {
		t.Fatal("expected not due immediately after touch")
	}

	// backdate the file mtime so it appears old → due again
	old := time.Now().Add(-10 * time.Minute)
	tsPath := filepath.Join(dir, staleCheckFilename)
	if err := os.Chtimes(tsPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if !staleCheckDue(dir, 5*time.Minute) {
		t.Fatal("expected due after file is backdated past interval")
	}
}

func TestTouchStaleCheck(t *testing.T) {
	dir := t.TempDir()
	touchStaleCheck(dir)

	info, err := os.Stat(filepath.Join(dir, staleCheckFilename))
	if err != nil {
		t.Fatalf("stat after touch: %v", err)
	}
	if time.Since(info.ModTime()) > 2*time.Second {
		t.Fatalf("mtime too old after touch: %v", info.ModTime())
	}
}

func TestOpenWithNilStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	a := &app{}
	if err := a.open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	if a.st == nil {
		t.Fatal("st should be non-nil after open()")
	}
	defer func() { _ = a.st.Close() }()

	// Second open is idempotent.
	st := a.st
	if err := a.open(); err != nil {
		t.Fatalf("second open: %v", err)
	}
	if a.st != st {
		t.Fatal("second open should not replace existing store")
	}
}
