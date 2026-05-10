package notify

import (
	"errors"
	"fmt"
	"os/exec"
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
// the notify-send error if the send itself fails. Returns nil when not requested
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
	return exec.Command("notify-send", title, body).Run()
}

func formatDuration(ms *int64) string {
	if ms == nil {
		return "unknown"
	}
	return (time.Duration(*ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}
