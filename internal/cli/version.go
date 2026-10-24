package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/version"
)

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "show carrier version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			t := newTheme(out)
			v := version.Current()
			besideLogo(out, logo(out), 5,
				fmt.Sprintf("%s %s", t.Accent.Bold(true).Render("carrier"), t.ID.Render(v.Version)),
				fmt.Sprintf("%s  %s", t.Bold.Render("commit:"), t.Muted.Render(v.Commit)),
				fmt.Sprintf("%s    %s", t.Bold.Render("date:"), t.Muted.Render(v.Date)))
			printHint(cmd, "update with: carrier self update")
			return nil
		},
	}
}
