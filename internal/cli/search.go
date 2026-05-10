package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) searchCmd() *cobra.Command {
	var jsonOutput bool
	var limit int
	cmd := &cobra.Command{
		Use:   "search <text>...",
		Short: "search commands and output",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := a.st.SearchRuns(strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if jsonOutput {
				type searchView struct {
					runView
					Snippet string `json:"snippet,omitempty"`
				}
				views := make([]searchView, 0, len(results))
				for _, result := range results {
					views = append(views, searchView{
						runView: runViewFromStore(&result.Run, false),
						Snippet: cleanSnippet(result.Snippet),
					})
				}
				return writeJSON(cmd, views)
			}
			out := cmd.OutOrStdout()
			c := outputColors(out)
			for _, result := range results {
				r := result.Run
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s\n",
					c.paint(colorCyan, fmt.Sprintf("%d", r.ID)),
					c.paint(statusColor(r.Status), r.Status),
					c.paint(colorGreen, displayCommand(&r)),
					c.paint(colorGray, r.CWD),
				)
				if snippet := cleanSnippet(result.Snippet); snippet != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", c.paint(colorGray, snippet))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "maximum number of results to show")
	return cmd
}

func cleanSnippet(snippet string) string {
	return strings.Join(strings.Fields(snippet), " ")
}
