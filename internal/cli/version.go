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
			c := outputColors(out)
			_, _ = fmt.Fprintln(out, c.paint(colorGreen, version.Current().String()))
			return nil
		},
	}
}
