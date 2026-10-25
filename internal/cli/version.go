package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/changelog"
	"github.com/atbuy/carrier/internal/selfupdate"
	"github.com/atbuy/carrier/internal/version"
)

// versionCheckTimeout bounds the check for a newer release, so "carrier
// version" stays quick when offline or on a slow network.
const versionCheckTimeout = 2 * time.Second

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "show carrier version",
		Long: `Show the carrier version, the commit it was built from, and when.

For a release build, it also checks for a newer release and says how to
update when there is one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			t := newTheme(out)
			v := version.Current()
			besideLogo(out, logo(out), 5,
				fmt.Sprintf("%s %s", t.Accent.Bold(true).Render("carrier"), t.ID.Render(v.Version)),
				fmt.Sprintf("%s  %s", t.Bold.Render("commit:"), t.Muted.Render(v.Commit)),
				fmt.Sprintf("%s    %s", t.Bold.Render("date:"), t.Muted.Render(v.Date)))
			if latest := newerRelease(cmd, v.Version); latest != "" {
				printHint(cmd, fmt.Sprintf("carrier %s is available; update with: carrier self update", plainVersion(latest)))
			}
			return nil
		},
	}
}

// newerRelease returns the latest release when it is newer than current, and
// "" otherwise: for a build that is not a release, when the latest release
// cannot be read in time, or when current is up to date.
func newerRelease(cmd *cobra.Command, current string) string {
	if !changelog.IsRelease(current) {
		return ""
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, versionCheckTimeout)
	defer cancel()
	latest, err := selfupdate.New().LatestVersion(ctx)
	if err != nil {
		return ""
	}
	if order, err := changelog.Compare(latest, current); err != nil || order <= 0 {
		return ""
	}
	return latest
}
