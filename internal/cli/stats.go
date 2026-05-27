package cli

import (
	"fmt"
	"io"
	"math"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func (a *app) statsCmd() *cobra.Command {
	var jsonOutput bool
	var slowestLimit int
	var commandPattern string
	cmd := &cobra.Command{
		Use:     "stats",
		Aliases: []string{"st"},
		Short:   "show run totals and slow commands",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := a.st.Stats(slowestLimit, commandPattern)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd, statsViewFromStore(stats))
			}
			printStats(cmd, stats, commandPattern)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVar(&slowestLimit, "slowest", 5, "number of slowest commands to show")
	cmd.Flags().StringVarP(&commandPattern, "command", "c", "", "filter by command substring")
	return cmd
}

func printStats(cmd *cobra.Command, stats *store.Stats, commandPattern string) {
	out := cmd.OutOrStdout()
	t := newTheme(out)
	if commandPattern != "" {
		_, _ = fmt.Fprintf(out, "%s%s\n\n", t.Bold.Render(padRight("Filter:", statsLabelWidth)), t.Command.Render(commandPattern))
	}
	printStatsLine(out, t, "Runs", fmt.Sprintf("%d", stats.TotalRuns), t.Bold)
	printStatsLine(out, t, "Completed", fmt.Sprintf("%d", stats.CompletedRuns), t.Bold)
	printStatsLine(out, t, "Success", fmt.Sprintf("%d", stats.SuccessfulRuns), t.Success)
	printStatsLine(out, t, "Failed", fmt.Sprintf("%d", stats.FailedRuns), nonZeroStyle(t, stats.FailedRuns, t.Danger))
	printStatsLine(out, t, "Running", fmt.Sprintf("%d", stats.RunningRuns), nonZeroStyle(t, stats.RunningRuns, t.Warning))
	printStatsLine(out, t, "Runs/day", fmt.Sprintf("%.2f", runsPerDay(stats)), t.Bold)
	printStatsLine(out, t, "Failure", fmt.Sprintf("%.1f%%", failureRate(stats)), failureStyle(t, stats))
	printStatsLine(out, t, "Avg duration", paddedDuration(stats.AvgDurationMS), t.Muted)
	printStatsLine(out, t, "Min duration", paddedDuration(stats.MinDurationMS), t.Muted)
	printStatsLine(out, t, "Max duration", paddedDuration(stats.MaxDurationMS), t.Muted)
	if stats.FirstStartedAt != nil {
		printStatsLine(out, t, "First run", timeWithRelative(*stats.FirstStartedAt), t.Muted)
	}
	if stats.LastStartedAt != nil {
		printStatsLine(out, t, "Last run", timeWithRelative(*stats.LastStartedAt), t.Muted)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, t.Bold.Render("Slowest:"))
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
			out, "  %s  %s  %s  %s  %s\n",
			t.ID.Render(fmt.Sprintf("%d", run.ID)),
			t.Muted.Render(formatDuration(&run.DurationMS)),
			renderStatus(t, run.Status, 0),
			t.Command.Render(displayCommand(storeRun)),
			t.Muted.Render(collapseHome(run.CWD)),
		)
	}
}

const statsLabelWidth = 13

func printStatsLine(out io.Writer, t theme, label, value string, style lipgloss.Style) {
	_, _ = fmt.Fprintf(out, "%s%s\n", t.Bold.Render(padRight(label+":", statsLabelWidth)), style.Render(value))
}

func nonZeroStyle(t theme, count int64, active lipgloss.Style) lipgloss.Style {
	if count == 0 {
		return t.Muted
	}
	return active
}

func failureStyle(t theme, stats *store.Stats) lipgloss.Style {
	if stats.FailedRuns == 0 {
		return t.Success
	}
	return t.Danger
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
	MinDurationMS  *int64        `json:"min_duration_ms,omitempty"`
	MaxDurationMS  *int64        `json:"max_duration_ms,omitempty"`
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
		MinDurationMS:  stats.MinDurationMS,
		MaxDurationMS:  stats.MaxDurationMS,
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
