package notify

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func TestFormatDuration(t *testing.T) {
	ms := int64(1234)
	if got := formatDuration(&ms); got != "1.2s" {
		t.Fatalf("formatDuration = %q", got)
	}
}

func TestFormatDurationUnknown(t *testing.T) {
	if got := formatDuration(nil); got != "unknown" {
		t.Fatalf("formatDuration(nil) = %q", got)
	}
}

// defaultCfg returns a config where both Success and Failure notifications are
// enabled, and the min_duration threshold is 10 s.
func defaultCfg() config.Config {
	return config.Default()
}

func TestMaybeSendNotRequested(t *testing.T) {
	r := store.Run{
		NotifyRequested: false,
		NotifyAlways:    false,
		Status:          store.StatusSuccess,
	}
	if err := MaybeSend(defaultCfg(), r); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestMaybeSendBelowThreshold(t *testing.T) {
	cfg := defaultCfg()
	// default min_duration is 10 s; use 1 ms so it is well below the threshold.
	durationMS := int64(1)
	r := store.Run{
		NotifyRequested: true,
		NotifyAlways:    false,
		Status:          store.StatusSuccess,
		DurationMS:      &durationMS,
	}
	if err := MaybeSend(cfg, r); err != ErrBelowThreshold {
		t.Fatalf("expected ErrBelowThreshold, got %v", err)
	}
}

func TestMaybeSendBelowThresholdNotifyAlwaysBypasses(t *testing.T) {
	// NotifyAlways should skip the threshold gate, but then be suppressed by
	// cfg.Notify.Success=false so we don't actually call notify-send.
	cfg := defaultCfg()
	cfg.Notify.Success = false

	durationMS := int64(1) // well below 10 s threshold
	r := store.Run{
		NotifyRequested: false,
		NotifyAlways:    true,
		Status:          store.StatusSuccess,
		DurationMS:      &durationMS,
	}
	// Should return nil (suppressed by Success=false), NOT ErrBelowThreshold.
	if err := MaybeSend(cfg, r); err != nil {
		t.Fatalf("expected nil (suppressed by Success=false), got %v", err)
	}
}

func TestMaybeSendSuccessSuppressedByConfig(t *testing.T) {
	cfg := defaultCfg()
	cfg.Notify.Success = false

	// Duration above threshold so threshold gate is passed.
	durationMS := int64(time.Minute.Milliseconds())
	r := store.Run{
		NotifyRequested: true,
		NotifyAlways:    false,
		Status:          store.StatusSuccess,
		DurationMS:      &durationMS,
	}
	if err := MaybeSend(cfg, r); err != nil {
		t.Fatalf("expected nil when success suppressed, got %v", err)
	}
}

func TestMaybeSendFailureSuppressedByConfig(t *testing.T) {
	cfg := defaultCfg()
	cfg.Notify.Failure = false

	durationMS := int64(time.Minute.Milliseconds())
	r := store.Run{
		NotifyRequested: true,
		NotifyAlways:    false,
		Status:          store.StatusFailed,
		DurationMS:      &durationMS,
	}
	if err := MaybeSend(cfg, r); err != nil {
		t.Fatalf("expected nil when failure suppressed, got %v", err)
	}
}

func TestMaybeSendNilDurationSkipsThresholdCheck(t *testing.T) {
	// When DurationMS is nil the threshold guard is skipped. Suppress by
	// Success=false so we don't invoke notify-send.
	cfg := defaultCfg()
	cfg.Notify.Success = false

	r := store.Run{
		NotifyRequested: true,
		NotifyAlways:    false,
		Status:          store.StatusSuccess,
		DurationMS:      nil,
	}
	if err := MaybeSend(cfg, r); err != nil {
		t.Fatalf("expected nil (suppressed by Success=false), got %v", err)
	}
}

func TestAvailableReturnsBool(t *testing.T) {
	_ = Available()
}

func TestAvailableChecksPlatformNotifier(t *testing.T) {
	oldGOOS := runtimeGOOS
	oldLookPath := execLookPath
	t.Cleanup(func() {
		runtimeGOOS = oldGOOS
		execLookPath = oldLookPath
	})

	tests := []struct {
		goos string
		want string
	}{
		{"darwin", "osascript"},
		{"windows", "powershell"},
		{"linux", "notify-send"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			runtimeGOOS = tt.goos
			execLookPath = func(name string) (string, error) {
				if name != tt.want {
					t.Fatalf("LookPath(%q), want %q", name, tt.want)
				}
				return "/bin/" + name, nil
			}
			if !Available() {
				t.Fatal("Available returned false")
			}
		})
	}

	runtimeGOOS = "linux"
	execLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	if Available() {
		t.Fatal("Available returned true for missing notifier")
	}
}

func TestSendPlatformBranches(t *testing.T) {
	oldGOOS := runtimeGOOS
	oldCommand := execCommand
	t.Cleanup(func() {
		runtimeGOOS = oldGOOS
		execCommand = oldCommand
	})

	execCommand = fakeNotifyCommand
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			runtimeGOOS = goos
			if err := send("title", "body"); err != nil {
				t.Fatalf("send: %v", err)
			}
		})
	}
}

func fakeNotifyCommand(command string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{"-test.run=TestNotifyHelperProcess", "--", command}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_NOTIFY_HELPER_PROCESS=1")
	return cmd
}

func TestNotifyHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_NOTIFY_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestMaybeSendReachesNotifier(t *testing.T) {
	// Exercises the send() path. notify-send may not be installed in CI;
	// we accept any error, but must not return ErrBelowThreshold.
	cfg := defaultCfg()
	cfg.Notify.Success = true

	durationMS := int64(time.Minute.Milliseconds())
	exitCode := 0
	r := store.Run{
		NotifyRequested: true,
		Status:          store.StatusSuccess,
		Command:         "go test ./...",
		DurationMS:      &durationMS,
		ExitCode:        &exitCode,
	}
	err := MaybeSend(cfg, r)
	if err == ErrBelowThreshold {
		t.Fatal("should not get ErrBelowThreshold when above threshold")
	}
}

func TestMaybeSendFailedCommandTitle(t *testing.T) {
	// Exercises the "carrier: command failed" title branch.
	cfg := defaultCfg()
	cfg.Notify.Failure = true

	durationMS := int64(time.Minute.Milliseconds())
	exitCode := 1
	r := store.Run{
		NotifyRequested: true,
		Status:          store.StatusFailed,
		Command:         "make lint",
		DurationMS:      &durationMS,
		ExitCode:        &exitCode,
	}
	err := MaybeSend(cfg, r)
	if err == ErrBelowThreshold {
		t.Fatal("should not get ErrBelowThreshold when above threshold")
	}
}

func TestMaybeSendNilExitCode(t *testing.T) {
	// Exercises the exit = "unknown" branch (ExitCode is nil).
	cfg := defaultCfg()
	cfg.Notify.Failure = true

	durationMS := int64(time.Minute.Milliseconds())
	r := store.Run{
		NotifyRequested: true,
		Status:          store.StatusFailed,
		DurationMS:      &durationMS,
	}
	err := MaybeSend(cfg, r)
	if err == ErrBelowThreshold {
		t.Fatal("should not get ErrBelowThreshold")
	}
}
