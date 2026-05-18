package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/config"
)

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "inspect and create config",
		Example: strings.Join([]string{
			"  carrier config path",
			"  carrier config init",
			"  carrier config check",
			"  carrier config show",
			"  carrier config init --force",
		}, "\n"),
	}
	cmd.AddCommand(configPathCmd(), configShowCmd(), configInitCmd(), configCheckCmd())
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
