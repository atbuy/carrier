package cli

import (
	"os"
	"path/filepath"
	"testing"
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
