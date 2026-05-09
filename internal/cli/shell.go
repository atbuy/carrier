package cli

import (
	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func (a *app) shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "start tracked shell (alpha)",
		Long:  "Start a PTY-backed tracked shell session. This command is alpha-quality and currently best-effort for zsh and bash.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return carriershell.Run(a.cfg, a.notify, a.notifyAlways, a.noRedact)
		},
	}
}
