package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) watchCmd() *cobra.Command {
	var (
		pattern  string
		debounce time.Duration
	)
	cmd := &cobra.Command{
		Use:   "watch <command...>",
		Short: "re-run command on file changes in CWD",
		Long: `Re-runs the command whenever files in the current directory change.

  carrier watch go test ./...
  carrier watch --pattern '*.go' go build ./...`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return fmt.Errorf("watch: %w", err)
			}
			defer func() { _ = watcher.Close() }()

			if err := addDirRecursive(watcher, cwd); err != nil {
				return err
			}

			if !a.quiet {
				_, _ = fmt.Fprintf(os.Stderr, "carrier: watching %s\n", cwd)
			}

			// run once immediately
			runWatch(a, args, cwd)

			var timer *time.Timer
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return nil
					}
					if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Remove) {
						continue
					}
					if pattern != "" {
						if matched, _ := filepath.Match(pattern, filepath.Base(event.Name)); !matched {
							continue
						}
					}
					if debounce > 0 {
						if timer != nil {
							timer.Stop()
						}
						timer = time.AfterFunc(debounce, func() {
							runWatch(a, args, cwd)
						})
					} else {
						runWatch(a, args, cwd)
					}
				case watchErr, ok := <-watcher.Errors:
					if !ok {
						return nil
					}
					if !a.quiet {
						_, _ = fmt.Fprintf(os.Stderr, "carrier: watch error: %v\n", watchErr)
					}
				}
			}
		},
	}
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "", "only react to files matching this glob pattern (e.g. '*.go')")
	cmd.Flags().DurationVarP(&debounce, "debounce", "d", 200*time.Millisecond, "wait this long after last change before re-running")
	return cmd
}

func runWatch(a *app, argv []string, cwd string) {
	_, _ = runner.Run(a.cfg, a.st, runner.Options{
		Mode:         store.ModeRun,
		Argv:         argv,
		CWD:          cwd,
		Notify:       a.notify,
		NotifyAlways: a.notifyAlways,
		NoRedact:     a.noRedact,
		Quiet:        a.quiet,
		Timeout:      a.timeout,
	})
}

func addDirRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}
