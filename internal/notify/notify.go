package notify

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/user/carrier/internal/config"
	"github.com/user/carrier/internal/store"
)

func MaybeSend(cfg config.Config, r store.Run) {
	if !r.NotifyRequested && !r.NotifyAlways {
		return
	}
	if r.DurationMS != nil && !r.NotifyAlways && time.Duration(*r.DurationMS)*time.Millisecond < cfg.NotifyMinDuration() {
		return
	}
	if r.Status == store.StatusSuccess && !cfg.Notify.Success {
		return
	}
	if r.Status != store.StatusSuccess && !cfg.Notify.Failure {
		return
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
	_ = exec.Command("notify-send", title, body).Run()
}

func formatDuration(ms *int64) string {
	if ms == nil {
		return "unknown"
	}
	return (time.Duration(*ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}
