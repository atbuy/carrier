package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/logs"
	"github.com/atbuy/carrier/internal/store"
)

const (
	staleCheckInterval = 5 * time.Minute
	staleCheckFilename = "stale-check.timestamp"
)

// staleCheckDue returns true when the stale-run cleanup should be executed.
// It checks whether the sidecar timestamp file is older than interval, which
// prevents the (usually no-op) UPDATE from firing on every carrier invocation.
func staleCheckDue(dataDir string, interval time.Duration) bool {
	return sidecarDue(dataDir, staleCheckFilename, interval)
}

func touchStaleCheck(dataDir string) {
	touchSidecar(dataDir, staleCheckFilename)
}

// sidecarDue reports whether the timestamp file name in dataDir is missing or
// older than interval, for throttling periodic maintenance.
func sidecarDue(dataDir, name string, interval time.Duration) bool {
	info, err := os.Stat(filepath.Join(dataDir, name))
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) >= interval
}

func touchSidecar(dataDir, name string) {
	_ = os.WriteFile(filepath.Join(dataDir, name), []byte(time.Now().UTC().Format(time.RFC3339)), 0o600)
}

// skipsStore reports whether a command runs without opening the database.
func skipsStore(path string) bool {
	switch path {
	case "carrier init":
		// init runs on every terminal start.
		return true
	case "carrier version":
		return true
	case "carrier changelog":
		// The deprecated alias of carrier self changelog.
		return true
	}
	// carrier self needs no database, and "self update" has to work even when
	// the database is broken.
	return strings.HasPrefix(path, "carrier self")
}

type app struct {
	cfg          config.Config
	cfgLoaded    bool
	st           *store.Store
	notify       bool
	notifyAlways bool
	quiet        bool
	noRedact     bool
	timeout      time.Duration
	label        string

	// completionTried is set once shell completion has tried to open the
	// store, so a failure is not retried for every candidate.
	completionTried bool
}

func Execute() int {
	root := newRootCmd(&app{})
	root.SetArgs(rewriteRunRefArgs(root, os.Args[1:]))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, newTheme(os.Stderr).Danger.Render("carrier:"), err)
		return 1
	}
	return 0
}

// newRootCmd builds the carrier command tree.
func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "carrier",
		Short:         "Local developer command logger",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Completion opens the store read-only itself, and must not fail
			// or write anything on a Tab press.
			if isCompletionCmd(cmd) {
				return nil
			}
			// Resolve color/theme settings for every command, including those
			// that skip store setup (version, config).
			if _, err := a.ensureConfig(); err != nil {
				return err
			}
			if skipsStore(cmd.CommandPath()) {
				return nil
			}
			if strings.HasPrefix(cmd.CommandPath(), "carrier config") || cmd.CommandPath() == "carrier setup" {
				return nil
			}
			if err := a.open(); err != nil {
				if auto, _ := cmd.Flags().GetBool("auto"); auto && cmd.Name() == "shell" {
					return autoStartError(true, err)
				}
				return err
			}
			return nil
		},
	}
	root.PersistentFlags().BoolVarP(&a.notify, "notify", "n", false, "request notification when command finishes")
	root.PersistentFlags().BoolVarP(&a.notifyAlways, "notify-always", "N", false, "notify regardless of duration threshold")
	root.PersistentFlags().BoolVarP(&a.quiet, "quiet", "q", false, "suppress carrier status output")
	root.PersistentFlags().BoolVar(&a.noRedact, "no-redact", false, "disable persisted log redaction for this run")

	root.AddCommand(a.runCmd(), a.lastCmd(), a.showCmd(), a.failedCmd(), a.runningCmd(), a.historyCmd(), a.hsCmd(), a.searchCmd(), a.similarCmd(), a.exportCmd(), a.rerunCmd(), a.statsCmd(), a.cleanCmd(), a.tailCmd(), a.watchCmd(), a.shellCmd(), a.initCmd(), a.setupCmd(), a.attachCmd(), a.detachCmd(), a.doctorCmd(), a.configCmd(), a.selfCmd(), a.versionCmd(), deprecatedSelfAlias(a.changelogCmd()), a.statusCmd(), a.internalCmd(), a.labelCmd(), a.noteCmd(), a.notesCmd(), a.sessionCmd(), a.browseCmd(), a.diffCmd(), a.completionCmd())
	installHelp(root)
	a.installCompletions(root)
	return root
}

// ensureConfig loads config once, caches it, and applies theme settings.
// Safe to call multiple times; subsequent calls are no-ops.
func (a *app) ensureConfig() (config.Config, error) {
	if a.cfgLoaded {
		return a.cfg, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return cfg, err
	}
	a.cfg = cfg
	a.cfgLoaded = true
	configureTheme(cfg)
	return cfg, nil
}

func (a *app) open() error {
	if a.st != nil {
		return nil
	}
	go logs.WarmBuiltinCache()
	cfg, err := a.ensureConfig()
	if err != nil {
		return err
	}
	st, err := store.OpenWith(cfg.Storage.DataDir, cfg)
	if err != nil {
		return err
	}
	a.st = st
	if staleCheckDue(cfg.Storage.DataDir, staleCheckInterval) {
		_, _ = st.MarkStaleRunsKilled(cfg.StaleRunThreshold())
		touchStaleCheck(cfg.Storage.DataDir)
	}
	return nil
}
