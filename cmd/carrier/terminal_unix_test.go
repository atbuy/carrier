//go:build !windows

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestNoBackgroundColorQuery runs carrier in a terminal that never answers
// queries. Bubble Tea 1.x asks the terminal for its background color at
// startup and waits up to 5 seconds for a reply; internal/termbg stops that.
// A dependency update that brings the query back fails here.
func TestNoBackgroundColorQuery(t *testing.T) {
	env := newCarrierTestEnv(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCarrierCLIHelperProcess$", "--", "version")
	cmd.Dir = env.work
	cmd.Env = append(
		os.Environ(),
		"CARRIER_TEST_CLI=1",
		"CARRIER_COLOR=",
		"NO_COLOR=",
		// termenv treats any CI value as "not a terminal", which would hide the
		// query this test looks for.
		"CI=",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"HOME="+env.home,
		"XDG_CONFIG_HOME="+env.xdgConfig,
	)

	start := time.Now()
	tty, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = tty.Close() }()

	var out bytes.Buffer
	read := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, tty) // ends with an error once carrier exits
		close(read)
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("carrier version: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("carrier version did not finish")
	}
	elapsed := time.Since(start)
	select {
	case <-read:
	case <-time.After(2 * time.Second):
	}

	if strings.Contains(out.String(), "\x1b]11;?") {
		t.Errorf("carrier asked the terminal for its background color:\n%q", out.String())
	}
	if elapsed > 3*time.Second {
		t.Errorf("carrier version took %v in a terminal that does not answer queries", elapsed)
	}
	// The logo still has its colors.
	if !strings.Contains(out.String(), "38;2;") {
		t.Errorf("want 24-bit colors in the output:\n%q", out.String())
	}
}
