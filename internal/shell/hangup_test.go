//go:build !windows

package shell

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
)

// TestRunHangsUpOnTheShell checks what closing a terminal does to a recorded
// shell: carrier gets SIGHUP, passes it on to the shell, and returns
// ErrHungUp with the shell's last state, instead of ending on the spot.
func TestRunHangsUpOnTheShell(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	program := filepath.Join(dir, "carrier-shell-test")
	// A command is in flight when the terminal closes.
	script := `#!/bin/sh
printf '{"current_id":7,"session_id":3}' > "$CARRIER_SHELL_STATE"
: > '` + started + `'
exec sleep 30
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Shell.Program = program

	type result struct {
		state State
		err   error
	}
	done := make(chan result, 1)
	go func() {
		state, err := Run(cfg, Options{SessionID: 3, Quiet: true})
		done <- result{state, err}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the shell did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if !errors.Is(r.err, ErrHungUp) {
			t.Fatalf("err = %v, want ErrHungUp", r.err)
		}
		if r.state.CurrentID != 7 {
			t.Fatalf("state = %+v, want the command in flight", r.state)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the hangup; the shell kept running")
	}
}
