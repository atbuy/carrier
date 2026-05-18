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
			_, _ = fmt.Fprintln(out, newTheme(out).Command.Render(version.Current().String()))
			return nil
		},
	}
}
