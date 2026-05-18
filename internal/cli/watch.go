package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/runner"
	"github.com/atbuy/carrier/internal/store"
)

// parseWatchFlags scans leading watch flags from args and returns the remaining
// args (the child command). Mirrors parseRunFlags so that DisableFlagParsing can
// be used on watchCmd, allowing arbitrary child-command flags to pass through.
func parseWatchFlags(args []string) (pattern string, debounce time.Duration, rest []string, err error) {
	debounce = 200 * time.Millisecond
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-p" || arg == "--pattern":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %q requires a value", arg)
			}
			pattern = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "--pattern="):
			pattern = strings.TrimPrefix(arg, "--pattern=")
			i++
		case strings.HasPrefix(arg, "-p="):
			pattern = strings.TrimPrefix(arg, "-p=")
			i++
		case arg == "-d" || arg == "--debounce":
			if i+1 >= len(args) {
				return "", 0, nil, fmt.Errorf("flag %q requires a value", arg)
			}
			d, parseErr := time.ParseDuration(args[i+1])
			if parseErr != nil {
				return "", 0, nil, fmt.Errorf("invalid debounce %q: %w", args[i+1], parseErr)
			}
			debounce = d
			i += 2
		case strings.HasPrefix(arg, "--debounce="):
			d, parseErr := time.ParseDuration(strings.TrimPrefix(arg, "--debounce="))
			if parseErr != nil {
				return "", 0, nil, fmt.Errorf("invalid debounce %q: %w", arg, parseErr)
			}
			debounce = d
			i++
		case strings.HasPrefix(arg, "-d="):
			d, parseErr := time.ParseDuration(strings.TrimPrefix(arg, "-d="))
			if parseErr != nil {
				return "", 0, nil, fmt.Errorf("invalid debounce %q: %w", arg, parseErr)
			}
			debounce = d
			i++
		default:
			return pattern, debounce, args[i:], nil
		}
	}
	return pattern, debounce, args[i:], nil
}

func (a *app) watchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "watch <command...>",
		Aliases:            []string{"w"},
		Short:              "re-run command on file changes in CWD",
		DisableFlagParsing: true,
		Long: `Re-runs the command whenever files in the current directory change.

  carrier watch go test ./...
  carrier watch --pattern '*.go' go build ./...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern, debounce, rest, err := parseWatchFlags(args)
			if err != nil {
				return err
			}
			if len(rest) == 0 {
				return cobra.MinimumNArgs(1)(cmd, rest)
			}

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
			runWatch(a, rest, cwd)

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
							runWatch(a, rest, cwd)
						})
					} else {
						runWatch(a, rest, cwd)
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
