package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func (a *app) labelCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "label <id> [text...]",
		Aliases: []string{"lb"},
		Short:   "set or clear a label on a run",
		Long: `Attach a short label to a run for later reference.

  carrier label 42 prod deploy   # set label "prod deploy"
  carrier label 42               # clear label`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			run, err := a.resolveRun(cmd, args[0])
			if err != nil {
				return err
			}
			id := run.ID
			label := strings.Join(args[1:], " ")
			if err := a.st.SetLabel(id, label); err != nil {
				return err
			}
			// Propagate label to the session this run belongs to, if any.
			if run.SessionID != nil {
				if err := a.st.UpdateSessionLabel(*run.SessionID, label); err == nil {
					carriershell.RelabelShells(a.cfg.Storage.DataDir, *run.SessionID, label)
				}
			}
			out := cmd.OutOrStdout()
			if label == "" {
				_, _ = fmt.Fprintf(out, "carrier: run %d label cleared\n", id)
			} else {
				_, _ = fmt.Fprintf(out, "carrier: run %d labeled %q\n", id, label)
			}
			return nil
		},
	}
}
