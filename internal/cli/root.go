package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

type app struct {
	cfg          config.Config
	st           *store.Store
	notify       bool
	notifyAlways bool
	quiet        bool
	noRedact     bool
	timeout      time.Duration
}

func Execute() int {
	a := &app{}
	root := &cobra.Command{
		Use:           "carrier",
		Short:         "Local developer command logger",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.CommandPath() == "carrier version" {
				return nil
			}
			if strings.HasPrefix(cmd.CommandPath(), "carrier config") {
				return nil
			}
			if cmd.CommandPath() == "carrier internal" || (cmd.Parent() != nil && cmd.Parent().Use == "internal") {
				return a.open()
			}
			return a.open()
		},
	}
	root.PersistentFlags().BoolVarP(&a.notify, "notify", "n", false, "request notification when command finishes")
	root.PersistentFlags().BoolVarP(&a.notifyAlways, "notify-always", "N", false, "notify regardless of duration threshold")
	root.PersistentFlags().BoolVarP(&a.quiet, "quiet", "q", false, "suppress carrier status output")
	root.PersistentFlags().BoolVar(&a.noRedact, "no-redact", false, "disable persisted log redaction for this run")

	root.AddCommand(a.runCmd(), a.lastCmd(), a.showCmd(), a.failedCmd(), a.runningCmd(), a.historyCmd(), a.searchCmd(), a.exportCmd(), a.rerunCmd(), a.statsCmd(), a.cleanCmd(), a.tailCmd(), a.watchCmd(), a.shellCmd(), a.attachCmd(), a.doctorCmd(), a.configCmd(), a.versionCmd(), a.internalCmd(), a.labelCmd(), a.sessionCmd())
	installHelp(root)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, newTheme(os.Stderr).Danger.Render("carrier:"), err)
		return 1
	}
	return 0
}

func (a *app) open() error {
	if a.st != nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := store.OpenWith(cfg.Storage.DataDir, cfg)
	if err != nil {
		return err
	}
	a.cfg = cfg
	a.st = st
	_, _ = st.MarkStaleRunsKilled(cfg.StaleRunThreshold())
	return nil
}
