package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
)

// retryRun runs the flaky test binary with --retry retries; it succeeds on
// run succeedOn, or never when that is 0.
func retryRun(t *testing.T, retries, succeedOn int) (code int, stderr string, runs []store.Run) {
	t.Helper()
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Default()
	cfg.Storage.DataDir = dir
	t.Setenv(flakyEnv, filepath.Join(t.TempDir(), "runs")+":"+strconv.Itoa(succeedOn))
	a := &app{st: st, cfg: cfg, retries: retries}
	cmd := &cobra.Command{}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)
	// The attempts' own "carrier: run N" lines go to the real stderr.
	code, err = a.runWithRetries(cmd, runner.Options{Mode: store.ModeRun, Argv: []string{os.Args[0]}, CWD: t.TempDir(), Quiet: false})
	if err != nil {
		t.Fatal(err)
	}
	runs, err = st.ListHistory(100, store.HistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return code, errOut.String(), runs
}

func TestRetryUntilSuccess(t *testing.T) {
	code, stderr, runs := retryRun(t, 3, 2)
	if code != 0 || len(runs) != 2 {
		t.Fatalf("code = %d, runs = %d", code, len(runs))
	}
	if runs[1].Status != store.StatusFailed || runs[0].Status != store.StatusSuccess {
		t.Errorf("statuses = %s then %s", runs[1].Status, runs[0].Status)
	}
	for _, want := range []string{
		"failed with exit 3; retrying (attempt 2 of 4)",
		"succeeded on attempt 2 of 4",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, stderr)
		}
	}
}

func TestRetryGivesUp(t *testing.T) {
	code, stderr, runs := retryRun(t, 2, 0)
	if code != 3 || len(runs) != 3 {
		t.Fatalf("code = %d, runs = %d", code, len(runs))
	}
	if !strings.Contains(stderr, "attempt 3 of 3") || !strings.Contains(stderr, "failed with exit 3; gave up after 3 attempts") {
		t.Errorf("stderr:\n%s", stderr)
	}
}

func TestRetryNotNeeded(t *testing.T) {
	code, stderr, runs := retryRun(t, 3, 1)
	if code != 0 || len(runs) != 1 || stderr != "" {
		t.Fatalf("code = %d, runs = %d, stderr = %q", code, len(runs), stderr)
	}
	// Without --retry a failure is run once.
	code, stderr, runs = retryRun(t, 0, 0)
	if code != 3 || len(runs) != 1 || stderr != "" {
		t.Fatalf("no retry: code = %d, runs = %d, stderr = %q", code, len(runs), stderr)
	}
}

func TestParseRetryFlags(t *testing.T) {
	a := &app{}
	rest, err := a.parseRunFlags([]string{"--retry", "3", "--retry-delay=5s", "make", "--retry", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.retries != 3 || a.retryDelay != 5*time.Second || strings.Join(rest, " ") != "make --retry 1" {
		t.Fatalf("retries = %d, delay = %v, rest = %v", a.retries, a.retryDelay, rest)
	}
	for _, bad := range [][]string{{"--retry", "-1", "x"}, {"--retry=many", "x"}, {"--retry-delay", "soon", "x"}, {"--retry"}} {
		if _, err := (&app{}).parseRunFlags(bad); err == nil {
			t.Errorf("%v should be refused", bad)
		}
	}
}

func TestRerunRetries(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	argv, _ := json.Marshal([]string{os.Args[0]})
	id := createFinishedRun(t, st, store.StatusFailed, "flaky", string(argv), t.TempDir())
	t.Setenv(flakyEnv, filepath.Join(t.TempDir(), "runs")+":3")
	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg, quiet: true}
	cmd := a.rerunCmd()
	var exitCode int
	oldExit := exitProcess
	exitProcess = func(code int) { exitCode = code }
	t.Cleanup(func() { exitProcess = oldExit })
	for flag, value := range map[string]string{"retry": "2", "retry-delay": "10ms"} {
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.RunE(cmd, []string{strconv.FormatInt(id, 10)}); err != nil {
		t.Fatal(err)
	}
	runs, _ := st.ListHistory(10, store.HistoryFilter{})
	var statuses []string
	for _, r := range runs {
		statuses = append(statuses, r.Mode+" "+r.Status)
	}
	// The original run, then three reruns, newest first.
	if want := "rerun success, rerun failed, rerun failed, run failed"; exitCode != 0 || strings.Join(statuses, ", ") != want {
		t.Fatalf("exit = %d, runs = %v, want %s", exitCode, statuses, want)
	}
}
