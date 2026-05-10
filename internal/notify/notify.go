package notify

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// ErrBelowThreshold is returned by MaybeSend when a notification was requested
// but suppressed because the command duration is below notify.min_duration.
// Use -N/--notify-always to bypass the threshold.
var ErrBelowThreshold = errors.New("duration below notify min_duration threshold (use -N/--notify-always to override)")

// MaybeSend sends a desktop notification if one was requested and all conditions
// are met. Returns ErrBelowThreshold when suppressed by the duration gate, or
// the send error if the platform notifier fails. Returns nil when not requested
// or when suppressed by config (success/failure flags).
func MaybeSend(cfg config.Config, r store.Run) error {
	if !r.NotifyRequested && !r.NotifyAlways {
		return nil
	}
	if r.DurationMS != nil && !r.NotifyAlways && time.Duration(*r.DurationMS)*time.Millisecond < cfg.NotifyMinDuration() {
		return ErrBelowThreshold
	}
	if r.Status == store.StatusSuccess && !cfg.Notify.Success {
		return nil
	}
	if r.Status != store.StatusSuccess && !cfg.Notify.Failure {
		return nil
	}
	title := "carrier: command finished"
	if r.Status != store.StatusSuccess {
		title = "carrier: command failed"
	}
	exit := "unknown"
	if r.ExitCode != nil {
		exit = fmt.Sprint(*r.ExitCode)
	}
	body := fmt.Sprintf("%s\nexit code: %s\nduration: %s", r.Command, exit, formatDuration(r.DurationMS))
	return send(title, body)
}

// send dispatches to the platform-appropriate notification command.
func send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(
			`display notification %q with title %q`,
			body, title,
		)
		return exec.Command("osascript", "-e", script).Run()
	case "windows":
		// PowerShell toast via BurntToast is optional; fall back to a simple
		// balloon via the Shell.Application COM object which ships with Windows.
		script := fmt.Sprintf(
			`[void][System.Reflection.Assembly]::LoadWithPartialName('System.Windows.Forms');`+
				`$n = New-Object System.Windows.Forms.NotifyIcon;`+
				`$n.Icon = [System.Drawing.SystemIcons]::Information;`+
				`$n.Visible = $true;`+
				`$n.ShowBalloonTip(5000, %q, %q, [System.Windows.Forms.ToolTipIcon]::None);`+
				`Start-Sleep -Milliseconds 5500;`+
				`$n.Dispose()`,
			title, body,
		)
		return exec.Command("powershell", "-NonInteractive", "-NoProfile", "-Command", script).Run()
	default:
		return exec.Command("notify-send", title, body).Run()
	}
}

// Available reports whether the platform notification tool is in PATH.
func Available() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("osascript")
		return err == nil
	case "windows":
		_, err := exec.LookPath("powershell")
		return err == nil
	default:
		_, err := exec.LookPath("notify-send")
		return err == nil
	}
}

func formatDuration(ms *int64) string {
	if ms == nil {
		return "unknown"
	}
	return (time.Duration(*ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}
