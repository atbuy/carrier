package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func (a *app) historyCmd() *cobra.Command {
	var (
		limit      int
		jsonOutput bool
		status     string
		since      string
		cwd        string
		branch     string
		command    string
		label      string
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "list recorded runs newest-first",
		Long: `List recorded runs newest-first, one per line.

Pipe to fzf to fuzzy-search and extract an ID for rerun:

  carrier history | fzf | awk '{print $1}' | xargs carrier rerun`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := store.HistoryFilter{Status: status, CWD: cwd, Branch: branch, Command: command, Label: label}
			if since != "" {
				d, err := parseAge(since)
				if err != nil {
					return fmt.Errorf("invalid --since duration %q: %w", since, err)
				}
				f.Since = time.Now().Add(-d)
			}
			runs, err := a.st.ListHistory(limit, f)
			if err != nil {
				return err
			}
			if jsonOutput {
				views := make([]runView, 0, len(runs))
				for _, r := range runs {
					views = append(views, runViewFromStore(&r, false))
				}
				return writeJSON(cmd, views)
			}
			out := cmd.OutOrStdout()
			c := outputColors(out)
			for _, r := range runs {
				labelSuffix := ""
				if r.Label != "" {
					labelSuffix = "  " + c.paint(colorMagenta, r.Label)
				}
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s  %s%s\n",
					c.paint(colorCyan, fmt.Sprintf("%6d", r.ID)),
					c.paint(statusColor(r.Status), padRight(r.Status, 7)),
					c.paint(colorGray, formatTime(r.StartedAt)),
					c.paint(colorGreen, displayCommand(&r)),
					c.paint(colorGray, r.CWD),
					labelSuffix,
				)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 500, "maximum number of runs to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (success, failed, running, killed)")
	cmd.Flags().StringVar(&since, "since", "", "only runs started within this duration ago (e.g. 24h, 7d)")
	cmd.Flags().StringVar(&cwd, "cwd", "", "filter by working directory (substring match)")
	cmd.Flags().StringVar(&branch, "branch", "", "filter by git branch (exact match)")
	cmd.Flags().StringVarP(&command, "command", "c", "", "filter by command (substring match)")
	cmd.Flags().StringVar(&label, "label", "", "filter by label (substring match)")
	return cmd
}
