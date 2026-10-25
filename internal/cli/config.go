package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/tui"
)

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config [command]",
		Aliases: []string{"cfg"},
		Short:   "change settings, or inspect and create config",
		Long: `Without a command, open the settings: scroll through every setting, toggle
it, pick one of its values, or type a new one. Each change is saved to the
config file right away, and carrier uses it from its next command. Comments
and everything else in the file stay as they are, so you can still edit the
file by hand.`,
		Example: strings.Join([]string{
			"  carrier config",
			"  carrier config set shell.branch_sessions follow",
			"  carrier config path",
			"  carrier config init",
			"  carrier config check",
			"  carrier config show",
			"  carrier config init --force",
		}, "\n"),
	}
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return cmd.Help()
		}
		path, err := config.Path()
		if err != nil {
			return err
		}
		if _, err := config.LoadFrom(path); err != nil {
			return fmt.Errorf("%s: %w\nfix the file by hand first; \"carrier config check\" checks it", collapseHome(path), err)
		}
		summary, err := tui.RunConfig(path)
		if err != nil {
			return err
		}
		if summary != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), summary)
		}
		return nil
	}
	cmd.AddCommand(configSetCmd(), configPathCmd(), configShowCmd(), configInitCmd(), configCheckCmd())
	return cmd
}

func configPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "show config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "show active config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return toml.NewEncoder(cmd.OutOrStdout()).Encode(cfg)
		},
	}
}

func configInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "write default config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("config already exists: %s", path)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(defaultConfigTOML()), 0o600); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			t2 := newTheme(out)
			_, _ = fmt.Fprintf(out, "%s %s\n", t2.Success.Render("wrote"), t2.Muted.Render(path))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite existing config")
	return cmd
}

func configCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "validate active config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			issues := config.Check(cfg)
			errors, warnings := config.CountIssues(issues)
			out := cmd.OutOrStdout()
			t := newTheme(out)
			printField(out, t, "Config", path, t.Muted)
			for _, issue := range issues {
				_, _ = fmt.Fprintf(
					out, "%s %s: %s\n",
					issueStyle(t, issue.Level).Render(issue.Level),
					t.Bold.Render(issue.Field),
					issue.Message,
				)
			}
			switch {
			case errors > 0:
				_, _ = fmt.Fprintf(out, "%s: %s\n", t.Danger.Render("failed"), issueSummary(errors, warnings))
				return fmt.Errorf("config check failed")
			case warnings > 0:
				_, _ = fmt.Fprintf(out, "%s: %s\n", t.Warning.Render("ok"), issueSummary(errors, warnings))
			default:
				_, _ = fmt.Fprintln(out, t.Success.Render("ok"))
			}
			return nil
		},
	}
}

func issueStyle(t theme, level string) lipgloss.Style {
	switch level {
	case config.IssueError:
		return t.Danger
	case config.IssueWarn:
		return t.Warning
	default:
		return t.Muted
	}
}

func issueSummary(errors, warnings int) string {
	return fmt.Sprintf("%d %s, %d %s", errors, plural("error", errors), warnings, plural("warning", warnings))
}

func plural(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func defaultConfigTOML() string {
	var buf bytes.Buffer
	_ = toml.NewEncoder(&buf).Encode(config.Default())
	return buf.String()
}

func configSetCmd() *cobra.Command {
	var add, remove, reset bool
	cmd := &cobra.Command{
		Use:   "set <key> <value>...",
		Short: "change one setting",
		Long: `Change one setting in the config file, as "carrier config" does, from the
command line or a script. Only the setting's line changes; comments and the
rest of the file stay as they are. A value "carrier config check" rejects is
not saved.

Toggles take true or false (also on, off, yes, no). A list takes each value as
an item and replaces the list; --add and --remove change single items.`,
		Example: strings.Join([]string{
			"  carrier config set shell.branch_sessions follow",
			"  carrier config set notify.min_duration 1m",
			"  carrier config set hints.similar_failures true",
			"  carrier config set shell.ignore_commands --add k9s",
			"  carrier config set ui.theme.accent '#BD93F9'",
			"  carrier config set --reset shell.branch_sessions",
		}, "\n"),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, ok := config.LookupSetting(args[0])
			if !ok {
				return fmt.Errorf("no setting %q; \"carrier config\" lists them", args[0])
			}
			path, err := config.Path()
			if err != nil {
				return err
			}
			if _, err := config.LoadFrom(path); err != nil {
				return fmt.Errorf("%s: %w\nfix the file by hand first; \"carrier config check\" checks it", collapseHome(path), err)
			}
			out := cmd.OutOrStdout()
			if reset {
				if add || remove || len(args) > 1 {
					return errors.New("--reset takes only the key")
				}
				file, err := config.Reset(path, s)
				if err != nil {
					return err
				}
				cfg, _ := config.LoadFrom(path)
				if file == "" {
					_, _ = fmt.Fprintf(out, "carrier: %s already has its default value, %s\n", s.Name(), config.Literal(s.Value(cfg)))
					return nil
				}
				_, _ = fmt.Fprintf(out, "carrier: reset %s to %s in %s\n", s.Name(), config.Literal(s.Value(cfg)), collapseHome(file))
				return nil
			}
			if add && remove {
				return errors.New("use --add or --remove, not both")
			}
			if (add || remove) && s.Kind != config.KindList {
				return fmt.Errorf("--add and --remove change lists; %s is not one", s.Name())
			}
			value, err := config.ParseValue(s, args[1:])
			if err != nil {
				return err
			}
			if add || remove {
				cfg, err := config.LoadFrom(path)
				if err != nil {
					return err
				}
				list := slices.Clone(s.Value(cfg).([]string))
				for _, item := range value.([]string) {
					if remove {
						if !slices.Contains(list, item) {
							return fmt.Errorf("%q is not in %s", item, s.Name())
						}
						list = slices.DeleteFunc(list, func(v string) bool { return v == item })
					} else if !slices.Contains(list, item) {
						list = append(list, item)
					}
				}
				value = list
			}
			file, err := config.Set(path, s, value)
			if err != nil {
				return fmt.Errorf("%s: %w", s.Name(), err)
			}
			_, _ = fmt.Fprintf(out, "carrier: set %s = %s in %s\n", s.Name(), config.Literal(value), collapseHome(file))
			return nil
		},
	}
	cmd.Flags().BoolVar(&add, "add", false, "add the values to a list")
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the values from a list")
	cmd.Flags().BoolVar(&reset, "reset", false, "remove the setting from the file, so its default applies")
	return cmd
}

// completeSettings completes "carrier config set": setting names with their
// titles, then the values a toggle or choice accepts.
func completeSettings(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		out := make([]cobra.Completion, 0, len(config.Settings))
		for _, s := range config.Settings {
			if strings.HasPrefix(s.Name(), toComplete) {
				out = append(out, cobra.CompletionWithDesc(s.Name(), s.Title))
			}
		}
		return out, noFiles
	}
	if f := cmd.Flags().Lookup("reset"); f != nil && f.Changed {
		return nil, noFiles
	}
	s, ok := config.LookupSetting(args[0])
	switch {
	case !ok:
		return nil, noFiles
	case s.Kind == config.KindBool && len(args) == 1:
		return []cobra.Completion{"true", "false"}, noFiles
	case s.Kind == config.KindChoice && len(args) == 1:
		return s.Choices, noFiles
	case s.Name() == "storage.data_dir" && len(args) == 1:
		return nil, cobra.ShellCompDirectiveFilterDirs
	case s.Name() == "shell.program" && len(args) == 1:
		return nil, cobra.ShellCompDirectiveDefault
	}
	return nil, noFiles
}
