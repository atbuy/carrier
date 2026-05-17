package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func TestShellCmdRunsConfiguredProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	program := filepath.Join(dir, "carrier-shell-cli-test")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf 'cli shell\\n'\n"), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}

	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Shell.Program = program

	st, err := store.Open(cfg.Storage.DataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{cfg: cfg, st: st}
	cmd := a.shellCmd()
	if err := cmd.Args(cmd, []string{"extra"}); err == nil {
		t.Fatal("expected no-args validation error")
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("shell command: %v", err)
	}
}
