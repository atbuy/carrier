package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <text>",
		Short: "search commands and output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := a.st.SearchRuns(args[0], 100)
			if err != nil {
				return err
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
}

func cleanSnippet(snippet string) string {
	return strings.Join(strings.Fields(snippet), " ")
}
