package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier"
	"github.com/atbuy/carrier/internal/changelog"
	"github.com/atbuy/carrier/internal/version"
)

const changelogWrapWidth = 88

func (a *app) changelogCmd() *cobra.Command {
	var all, jsonOutput, markdown bool
	var since string
	cmd := &cobra.Command{
		Use:     "changelog [version]",
		Aliases: []string{"news"},
		Short:   "show what changed in each version",
		Long: `Show what changed in a carrier version. Without arguments, it shows the
installed version.

  carrier changelog               # the installed version
  carrier changelog 0.24.0        # one version
  carrier changelog --since 0.20.0  # everything newer than 0.20.0
  carrier changelog --all         # every version
  carrier changelog --markdown 0.24.0  # Markdown, e.g. for release notes`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			releases, err := changelog.Parse(carrier.Changelog)
			if err != nil {
				return fmt.Errorf("built-in changelog: %w", err)
			}
			if len(releases) == 0 {
				return fmt.Errorf("built-in changelog is empty")
			}
			if len(args) == 1 && (all || since != "") {
				return fmt.Errorf("give a version, --since, or --all, not more than one")
			}
			if all && since != "" {
				return fmt.Errorf("use --since or --all, not both")
			}

			var shown []changelog.Release
			note := ""
			switch {
			case all:
				shown = releases
			case since != "":
				shown, err = changelog.Since(releases, since)
				if err != nil {
					return err
				}
				if len(shown) == 0 {
					note = fmt.Sprintf("no versions newer than %s", strings.TrimPrefix(since, "v"))
				}
			case len(args) == 1:
				r, ok := changelog.Find(releases, args[0])
				if !ok {
					return fmt.Errorf("no changelog entry for version %s; see carrier changelog --all", strings.TrimPrefix(args[0], "v"))
				}
				shown = []changelog.Release{r}
			default:
				current := version.Current().Version
				if r, ok := changelog.Find(releases, current); ok {
					shown = []changelog.Release{r}
				} else {
					shown = releases[:1]
					if !changelog.IsRelease(current) {
						note = fmt.Sprintf("this is a %s build; showing the latest release", current)
					} else {
						note = fmt.Sprintf("no entry for %s; showing the latest release", strings.TrimPrefix(current, "v"))
					}
				}
			}

			if markdown {
				for i, r := range shown {
					if len(shown) > 1 {
						if i > 0 {
							_, _ = fmt.Fprintln(cmd.OutOrStdout())
						}
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "## %s\n\n", r.Version)
					}
					_, _ = fmt.Fprint(cmd.OutOrStdout(), changelog.Markdown(r))
				}
				return nil
			}
			if jsonOutput {
				if shown == nil {
					shown = []changelog.Release{}
				}
				return writeJSON(cmd, shown)
			}
			if note != "" {
				printHint(cmd, note)
			}
			out := cmd.OutOrStdout()
			t := newTheme(out)
			for i, r := range shown {
				if i > 0 {
					_, _ = fmt.Fprintln(out)
				}
				printRelease(out, t, r)
			}
			if len(args) == 0 && !all && since == "" {
				printHint(cmd, `older versions: carrier changelog --all, or --since <version>`)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "show every version")
	cmd.Flags().StringVar(&since, "since", "", "show versions newer than this one")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "output Markdown")
	cmd.MarkFlagsMutuallyExclusive("json", "markdown")
	return cmd
}

func printRelease(w io.Writer, t theme, r changelog.Release) {
	heading := t.Accent.Bold(true).Render("carrier " + r.Version)
	if r.Date != "" {
		heading += " " + t.Muted.Render(r.Date)
	}
	_, _ = fmt.Fprintln(w, heading)
	for _, sec := range r.Sections {
		_, _ = fmt.Fprintln(w, "  "+t.Bold.Render(sec.Name))
		for _, item := range sec.Items {
			// Code spans and bold read fine without their Markdown markers.
			plain := strings.NewReplacer("`", "", "**", "").Replace(item)
			for j, line := range wrapWords(plain, changelogWrapWidth-6) {
				prefix := "    • "
				if j > 0 {
					prefix = "      "
				}
				_, _ = fmt.Fprintln(w, prefix+line)
			}
		}
	}
}

// wrapWords breaks text into lines of at most width characters, splitting
// only at spaces.
func wrapWords(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
