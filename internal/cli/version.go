package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/version"
)

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "show carrier version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			t := newTheme(out)
			v := version.Current()
			_, _ = fmt.Fprintf(out, "%s %s\n", t.Accent.Bold(true).Render("carrier"), t.ID.Render(v.Version))
			_, _ = fmt.Fprintf(out, "%s  %s\n", t.Bold.Render("commit:"), t.Muted.Render(v.Commit))
			_, _ = fmt.Fprintf(out, "%s    %s\n", t.Bold.Render("date:"), t.Muted.Render(v.Date))
			return nil
		},
	}
}
