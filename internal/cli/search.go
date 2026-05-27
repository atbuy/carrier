package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

func (a *app) searchCmd() *cobra.Command {
	var jsonOutput bool
	var limit int
	var since string
	var status string
	cmd := &cobra.Command{
		Use:     "search <text>...",
		Aliases: []string{"sr"},
		Short:   "search commands and output",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := store.SearchFilter{Status: status}
			if since != "" {
				d, err := parseSinceDuration(since)
				if err != nil {
					return fmt.Errorf("--since: %w", err)
				}
				t := time.Now().Add(-d)
				f.Since = &t
			}
			results, err := a.st.SearchRuns(strings.Join(args, " "), limit, f)
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
			t := newTheme(out)
			for _, result := range results {
				r := result.Run
				_, _ = fmt.Fprintf(
					out, "%s  %s  %s  %s\n",
					t.ID.Render(fmt.Sprintf("%d", r.ID)),
					t.statusStyle(r.Status).Render(r.Status),
					t.Command.Render(displayCommand(&r)),
					t.Muted.Render(r.CWD),
				)
				if snippet := cleanSnippet(result.Snippet); snippet != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", t.Muted.Render(snippet))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().IntVarP(&limit, "limit", "l", 100, "maximum number of results to show")
	cmd.Flags().StringVar(&since, "since", "", "only show runs started within duration (e.g. 1h, 24h, 7d)")
	cmd.Flags().StringVar(&status, "status", "", "only show runs with this status (success, failed, killed, running)")
	return cmd
}

func cleanSnippet(snippet string) string {
	return strings.Join(strings.Fields(snippet), " ")
}
