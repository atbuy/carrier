package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) shellCmd() *cobra.Command {
	var label string
	cmd := &cobra.Command{
		Use:     "shell",
		Aliases: []string{"s"},
		Short:   "start tracked shell (alpha)",
		Long:    "Start a PTY-backed tracked shell session. This command is alpha-quality and currently best-effort for zsh and bash.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && label == "" {
				label = args[0]
			}
			sessionID, err := a.st.CreateSession(store.CreateSession{
				Label:     strings.TrimSpace(label),
				StartedAt: time.Now(),
			})
			if err != nil {
				return err
			}
			runErr := carriershell.Run(a.cfg, a.notify, a.notifyAlways, a.noRedact, sessionID)
			_ = a.st.EndSession(sessionID, time.Now())
			return runErr
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "label for this shell session")
	return cmd
}
