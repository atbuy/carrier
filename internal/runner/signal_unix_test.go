//go:build !windows

package runner

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// TestRunRecordsStoppedCommands checks that Ctrl+C, or a termination sent to
// carrier, records the run as killed instead of leaving it running, and that
// a termination is passed on to the command.
func TestRunRecordsStoppedCommands(t *testing.T) {
	cases := []struct {
		name     string
		sig      syscall.Signal
		sleep    string
		wantExit int
	}{
		// Ctrl+C reaches the command from the terminal, not from carrier; here
		// only carrier gets it, so the command finishes its sleep.
		{"interrupt", syscall.SIGINT, "1", 0},
		{"terminate", syscall.SIGTERM, "30", 128 + int(syscall.SIGTERM)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dataDir := t.TempDir()
			st, err := store.Open(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			cfg := config.Default()
			cfg.Storage.DataDir = dataDir
			cfg.Notify.Failure = true
			mark := filepath.Join(t.TempDir(), "started")

			go func() {
				for i := 0; i < 500; i++ {
					if _, err := os.Stat(mark); err == nil {
						// carrier listens for signals right after starting the command.
						time.Sleep(200 * time.Millisecond)
						_ = syscall.Kill(os.Getpid(), c.sig)
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			}()
			code, err := Run(cfg, st, Options{
				Mode: store.ModeRun, Argv: []string{"sh", "-c", "touch '" + mark + "'; exec sleep " + c.sleep},
				CWD: t.TempDir(), Quiet: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if code != c.wantExit {
				t.Errorf("exit = %d, want %d", code, c.wantExit)
			}
			r, err := st.Latest()
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != store.StatusKilled {
				t.Errorf("status = %q, want killed", r.Status)
			}
		})
	}
}
