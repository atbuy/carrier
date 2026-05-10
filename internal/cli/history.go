package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *app) historyCmd() *cobra.Command {
	var (
		limit      int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "list recorded runs newest-first",
		Long: `List recorded runs newest-first, one per line.

Pipe to fzf to fuzzy-search and extract an ID for rerun:

  carrier history | fzf | awk '{print $1}' | xargs carrier rerun`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := a.st.All(limit)
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
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s  %s\n",
					c.paint(colorCyan, fmt.Sprintf("%6d", r.ID)),
					c.paint(statusColor(r.Status), padRight(r.Status, 7)),
					c.paint(colorGray, formatTime(r.StartedAt)),
					c.paint(colorGreen, displayCommand(&r)),
					c.paint(colorGray, r.CWD),
				)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 500, "maximum number of runs to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}
