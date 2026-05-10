package cli

import (
	"fmt"
	"io"
	"math"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func (a *app) statsCmd() *cobra.Command {
	var jsonOutput bool
	var slowestLimit int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "show run totals and slow commands",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := a.st.Stats(slowestLimit)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd, statsViewFromStore(stats))
			}
			printStats(cmd, stats)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVar(&slowestLimit, "slowest", 5, "number of slowest commands to show")
	return cmd
}

func printStats(cmd *cobra.Command, stats *store.Stats) {
	out := cmd.OutOrStdout()
	c := helpColors{enabled: shouldColor(out)}
	printStatsLine(out, c, "Runs", fmt.Sprintf("%d", stats.TotalRuns), "")
	printStatsLine(out, c, "Completed", fmt.Sprintf("%d", stats.CompletedRuns), "")
	printStatsLine(out, c, "Success", fmt.Sprintf("%d", stats.SuccessfulRuns), colorGreen)
	printStatsLine(out, c, "Failed", fmt.Sprintf("%d", stats.FailedRuns), nonZeroColor(stats.FailedRuns, colorRed))
	printStatsLine(out, c, "Running", fmt.Sprintf("%d", stats.RunningRuns), nonZeroColor(stats.RunningRuns, colorYellow))
	printStatsLine(out, c, "Runs/day", fmt.Sprintf("%.2f", runsPerDay(stats)), colorCyan)
	printStatsLine(out, c, "Failure", fmt.Sprintf("%.1f%%", failureRate(stats)), failureColor(stats))
	printStatsLine(out, c, "Avg duration", paddedDuration(stats.AvgDurationMS), colorCyan)
	if stats.FirstStartedAt != nil {
		printStatsLine(out, c, "First run", formatTime(*stats.FirstStartedAt), colorGray)
	}
	if stats.LastStartedAt != nil {
		printStatsLine(out, c, "Last run", formatTime(*stats.LastStartedAt), colorGray)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, c.paint(colorBold, "Slowest:"))
	if len(stats.SlowestRuns) == 0 {
		_, _ = fmt.Fprintln(out, "  none")
		return
	}
	for _, run := range stats.SlowestRuns {
		storeRun := &store.Run{
			ID:         run.ID,
			Status:     run.Status,
			Mode:       run.Mode,
			Command:    run.Command,
			ArgvJSON:   run.ArgvJSON,
			CWD:        run.CWD,
			StartedAt:  run.StartedAt,
			DurationMS: &run.DurationMS,
		}
		_, _ = fmt.Fprintf(
			out, "  %d  %s  %s  %s  %s\n",
			run.ID,
			c.paint(colorCyan, formatDuration(&run.DurationMS)),
			c.paint(statusColor(run.Status), run.Status),
			displayCommand(storeRun),
			c.paint(colorGray, run.CWD),
		)
	}
}

const statsLabelWidth = 13

func printStatsLine(out io.Writer, c helpColors, label, value, valueColor string) {
	if valueColor != "" {
		value = c.paint(valueColor, value)
	}
	_, _ = fmt.Fprintf(out, "%s%s\n", c.paint(colorBold, padRight(label+":", statsLabelWidth)), value)
}

func nonZeroColor(count int64, color string) string {
	if count == 0 {
		return ""
	}
	return color
}

func failureColor(stats *store.Stats) string {
	if stats.FailedRuns == 0 {
		return colorGreen
	}
	return colorRed
}

func runsPerDay(stats *store.Stats) float64 {
	if stats.ActiveDays <= 0 {
		return 0
	}
	return float64(stats.TotalRuns) / float64(stats.ActiveDays)
}

func failureRate(stats *store.Stats) float64 {
	if stats.CompletedRuns <= 0 {
		return 0
	}
	return float64(stats.FailedRuns) * 100 / float64(stats.CompletedRuns)
}

func paddedDuration(ms *int64) string {
	if ms == nil {
		return " "
	}
	return " " + formatDuration(ms)
}

type statsView struct {
	TotalRuns      int64         `json:"total_runs"`
	CompletedRuns  int64         `json:"completed_runs"`
	SuccessfulRuns int64         `json:"successful_runs"`
	FailedRuns     int64         `json:"failed_runs"`
	RunningRuns    int64         `json:"running_runs"`
	ActiveDays     int64         `json:"active_days"`
	RunsPerDay     float64       `json:"runs_per_day"`
	FailureRate    float64       `json:"failure_rate"`
	AvgDurationMS  *int64        `json:"avg_duration_ms,omitempty"`
	SlowestRuns    []slowRunView `json:"slowest_runs"`
}

type slowRunView struct {
	ID         int64    `json:"id"`
	Status     string   `json:"status"`
	Mode       string   `json:"mode"`
	Command    string   `json:"command"`
	Argv       []string `json:"argv,omitempty"`
	CWD        string   `json:"cwd"`
	DurationMS int64    `json:"duration_ms"`
}

func statsViewFromStore(stats *store.Stats) statsView {
	view := statsView{
		TotalRuns:      stats.TotalRuns,
		CompletedRuns:  stats.CompletedRuns,
		SuccessfulRuns: stats.SuccessfulRuns,
		FailedRuns:     stats.FailedRuns,
		RunningRuns:    stats.RunningRuns,
		ActiveDays:     stats.ActiveDays,
		RunsPerDay:     roundFloat(runsPerDay(stats), 2),
		FailureRate:    roundFloat(failureRate(stats), 1),
		AvgDurationMS:  stats.AvgDurationMS,
		SlowestRuns:    make([]slowRunView, 0, len(stats.SlowestRuns)),
	}
	for _, run := range stats.SlowestRuns {
		argv, _ := parseArgv(run.ArgvJSON)
		storeRun := &store.Run{Mode: run.Mode, Command: run.Command, ArgvJSON: run.ArgvJSON}
		view.SlowestRuns = append(view.SlowestRuns, slowRunView{
			ID:         run.ID,
			Status:     run.Status,
			Mode:       run.Mode,
			Command:    displayCommand(storeRun),
			Argv:       argv,
			CWD:        run.CWD,
			DurationMS: run.DurationMS,
		})
	}
	return view
}

func roundFloat(v float64, places int) float64 {
	pow := math.Pow10(places)
	return math.Round(v*pow) / pow
}
