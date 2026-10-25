package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	carrier "github.com/atbuy/carrier"
	"github.com/atbuy/carrier/internal/changelog"
	"github.com/atbuy/carrier/internal/store"
)

func (a *app) completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "generate shell completion script",
		Long: `Generate a shell completion script for carrier.

"carrier init" already sets up completion in zsh, bash, and fish, so this is
only needed without it.

BASH
  # Write once, then restart your shell:
  carrier completion bash > /etc/bash_completion.d/carrier

  # Or source per-session (add to ~/.bashrc for persistence):
  source <(carrier completion bash)

ZSH
  # Add to a directory in $fpath, then restart your shell:
  carrier completion zsh > "${fpath[1]}/_carrier"

  # If completions are not already enabled, add to ~/.zshrc:
  autoload -U compinit && compinit

FISH
  # Write once (fish loads completions automatically):
  carrier completion fish > ~/.config/fish/completions/carrier.fish

POWERSHELL
  # Add to your PowerShell profile ($PROFILE):
  carrier completion powershell | Out-String | Invoke-Expression`,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash", "fish":
				// The same as "carrier init" sets up: also for the "c"
				// alias, and completing the command after run and watch.
				script, err := completionScript(root, args[0])
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), script)
				return err
			case "zsh":
				// A plain #compdef file for $fpath.
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
			return nil
		},
	}
	return cmd
}

// completionLimit is how many runs, sessions, or values one Tab offers.
const completionLimit = 20

// noFiles is the directive for completions that never name files. KeepOrder
// keeps the newest-first order instead of letting the shell sort by name.
const noFiles = cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder

// completionStore opens the store read-only for shell completion, once. It
// returns nil when there is no database or it cannot be read, so completion
// offers nothing rather than printing an error into the prompt.
func (a *app) completionStore() *store.Store {
	if a.st != nil {
		return a.st
	}
	if a.completionTried {
		return nil
	}
	a.completionTried = true
	cfg, err := a.ensureConfig()
	if err != nil {
		return nil
	}
	st, err := store.OpenReadOnly(cfg.Storage.DataDir)
	if err != nil {
		return nil
	}
	a.st = st
	return st
}

// isCompletionCmd reports whether cmd is cobra's hidden command that shells
// call on Tab.
func isCompletionCmd(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || cmd.Name() == cobra.ShellCompNoDescRequestCmd
}

// completionSpecs says what Tab offers: args by command path, flags by
// command path and flag name, and the flags that take a path, which keep the
// shell's file completion.
type completionSpecs struct {
	args      map[string]cobra.CompletionFunc
	flags     map[string]map[string]cobra.CompletionFunc
	fileFlags map[string]bool
}

// installCompletions sets what Tab offers for every argument and flag. A
// command or flag without a specific completion offers nothing, rather than
// the shell's file names, unless it takes a path.
func (a *app) installCompletions(root *cobra.Command) {
	specs := a.completionSpecs()
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := cmd.CommandPath()
		switch {
		case specs.args[path] != nil:
			cmd.ValidArgsFunction = specs.args[path]
		case cmd.ValidArgsFunction == nil && len(cmd.ValidArgs) == 0:
			// run and watch take a command line; the shell scripts from
			// "carrier init" complete it as if it were typed on its own.
			cmd.ValidArgsFunction = cobra.NoFileCompletions
		}
		cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || specs.fileFlags[path+" --"+f.Name] {
				return
			}
			if _, ok := cmd.GetFlagCompletionFunc(f.Name); ok {
				return
			}
			fn := specs.flags[path][f.Name]
			if fn == nil {
				fn = cobra.NoFileCompletions
			}
			_ = cmd.RegisterFlagCompletionFunc(f.Name, fn)
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func (a *app) completionSpecs() completionSpecs {
	runs := a.completeRuns(runCompletion{})
	sessions := a.completeSessions(sessionCompletion{})
	args := map[string]cobra.CompletionFunc{
		"carrier show":           onlyFirst(runs),
		"carrier tail":           onlyFirst(a.completeRuns(runCompletion{running: true})),
		"carrier export":         onlyFirst(runs),
		"carrier rerun":          onlyFirst(runs),
		"carrier label":          onlyFirst(runs),
		"carrier similar":        onlyFirst(runs),
		"carrier diff":           a.completeRuns(runCompletion{pairs: true}),
		"carrier attach":         unlessFlag("branch", onlyFirst(a.completeSessions(sessionCompletion{notCurrent: true}))),
		"carrier session branch": onlyFirst(sessions),
		"carrier session show":   onlyFirst(sessions),
		"carrier session export": onlyFirst(sessions),
		"carrier session end":    onlyFirst(sessions),
		"carrier session pin":    onlyFirst(sessions),
		"carrier session keep":   onlyFirst(a.completeSessions(sessionCompletion{kept: ptr(false)})),
		"carrier session unkeep": onlyFirst(a.completeSessions(sessionCompletion{kept: ptr(true)})),
		"carrier session label":  onlyFirst(a.completeSessions(sessionCompletion{idsOnly: true})),
		"carrier notes rm":       onlyFirst(a.completeNotes),
		"carrier self changelog": onlyFirst(completeVersions),
		"carrier changelog":      onlyFirst(completeVersions),
		"carrier completion":     fixedValues("bash", "zsh", "fish", "powershell"),
		"carrier init":           fixedValues("zsh", "bash", "fish"),
		"carrier config set":     completeSettings,
	}
	statuses := fixedValues(store.StatusSuccess, store.StatusFailed, store.StatusRunning, store.StatusKilled)
	durations := completeDurations
	flags := map[string]map[string]cobra.CompletionFunc{
		"carrier history": {
			"status": statuses, "since": durations, "session": sessions,
			"branch": a.completeValues(store.ValueBranch), "label": a.completeValues(store.ValueLabel),
			"cwd": a.completeValues(store.ValueCWD), "command": a.completeValues(store.ValueCommand),
		},
		"carrier search":         {"status": statuses, "since": durations},
		"carrier stats":          {"command": a.completeValues(store.ValueCommand)},
		"carrier clean":          {"older-than": durations},
		"carrier tail":           {"stream": fixedValues("both", "stdout", "stderr", "terminal")},
		"carrier diff":           {"stream": fixedValues("auto", "stdout", "stderr", "terminal")},
		"carrier export":         {"format": fixedValues("markdown", "json", "csv")},
		"carrier session export": {"format": fixedValues("markdown", "json")},
		"carrier note":           {"run": runs, "session": sessions},
		"carrier notes":          {"run": runs, "session": sessions},
		"carrier setup":          {"shell": fixedValues("zsh", "bash", "fish")},
		"carrier self changelog": {"since": completeVersions},
		"carrier changelog":      {"since": completeVersions},
		"carrier self update":    {"version": completeVersions},
	}
	flags["carrier hs"] = flags["carrier history"]
	return completionSpecs{
		args:  args,
		flags: flags,
		fileFlags: map[string]bool{
			"carrier session export --output-file": true,
			"carrier shell --program":              true,
		},
	}
}

func ptr[T any](v T) *T { return &v }

// unlessFlag completes with fn unless the flag is set, which replaces the
// argument, as --branch does for attach.
func unlessFlag(flag string, fn cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if f := cmd.Flags().Lookup(flag); f != nil && f.Changed {
			return nil, noFiles
		}
		return fn(cmd, args, toComplete)
	}
}

// onlyFirst completes the first argument with fn and nothing after it.
func onlyFirst(fn cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, noFiles
		}
		return fn(cmd, args, toComplete)
	}
}

func fixedValues(values ...string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return values, noFiles
	}
}

// isDigits reports whether s is empty or only digits: a run or session ID
// being typed, rather than a label.
func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type runCompletion struct {
	// running offers only runs that are still running, for tail.
	running bool
	// pairs completes both arguments of diff, leaving out the first run
	// from the second.
	pairs bool
}

// completeRuns offers recent runs. While an ID is typed it inserts IDs, with
// the status, age, and command as the description; once letters are typed it
// inserts run labels, which every run reference also accepts.
func (a *app) completeRuns(opts runCompletion) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 1 || (len(args) == 1 && !opts.pairs) {
			return nil, noFiles
		}
		st := a.completionStore()
		if st == nil {
			return nil, noFiles
		}
		f := store.RunChoiceFilter{Running: opts.running, Limit: completionLimit}
		if len(args) == 1 {
			if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				f.Exclude = []int64{id}
			}
		}
		labels := !isDigits(toComplete)
		if labels {
			f.LabelPrefix = toComplete
		} else {
			f.IDPrefix = toComplete
		}
		choices, err := st.RunChoices(f)
		if err != nil {
			return nil, noFiles
		}
		var out []cobra.Completion
		seen := map[string]bool{}
		for _, c := range choices {
			if !labels {
				out = append(out, cobra.CompletionWithDesc(strconv.FormatInt(c.ID, 10), runChoiceDescription(c, true)))
				continue
			}
			if !seen[c.Label] {
				seen[c.Label] = true
				out = append(out, cobra.CompletionWithDesc(c.Label, fmt.Sprintf("run %d · %s", c.ID, runChoiceDescription(c, false))))
			}
		}
		return out, noFiles
	}
}

// runChoiceDescription describes a run next to its ID, or next to its label
// without repeating it.
func runChoiceDescription(c store.RunChoice, withLabel bool) string {
	parts := []string{c.Status, formatRelativeTime(c.StartedAt), shortText(c.Command, 60)}
	if withLabel && c.Label != "" {
		parts = append(parts, c.Label)
	}
	return strings.Join(parts, " · ")
}

type sessionCompletion struct {
	// notCurrent leaves out the session this shell records into, for attach.
	notCurrent bool
	// kept offers only kept sessions when true, and only unkept ones when
	// false.
	kept *bool
	// idsOnly offers sessions only while an ID is typed: "session label"
	// takes the label text after an optional ID.
	idsOnly bool
}

// completeSessions offers recent sessions the same way completeRuns offers
// runs: IDs while digits are typed, labels once letters are.
func (a *app) completeSessions(opts sessionCompletion) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		labels := !isDigits(toComplete)
		if labels && opts.idsOnly {
			return nil, noFiles
		}
		st := a.completionStore()
		if st == nil {
			return nil, noFiles
		}
		f := store.SessionChoiceFilter{Kept: opts.kept, Limit: completionLimit}
		if opts.notCurrent {
			if id := currentSessionID(); id != 0 {
				f.Exclude = []int64{id}
			}
		}
		if labels {
			f.LabelPrefix = toComplete
		} else {
			f.IDPrefix = toComplete
		}
		choices, err := st.SessionChoices(f)
		if err != nil {
			return nil, noFiles
		}
		var out []cobra.Completion
		seen := map[string]bool{}
		for _, c := range choices {
			if !labels {
				out = append(out, cobra.CompletionWithDesc(strconv.FormatInt(c.ID, 10), sessionChoiceDescription(c, true)))
				continue
			}
			if !seen[c.Label] {
				seen[c.Label] = true
				out = append(out, cobra.CompletionWithDesc(c.Label, fmt.Sprintf("session %d · %s", c.ID, sessionChoiceDescription(c, false))))
			}
		}
		return out, noFiles
	}
}

// sessionChoiceDescription describes a session next to its ID, or next to
// its label without repeating it.
func sessionChoiceDescription(c store.SessionChoice, withLabel bool) string {
	var parts []string
	switch {
	case !withLabel:
	case c.Label == "":
		parts = append(parts, "no label")
	default:
		parts = append(parts, c.Label)
	}
	parts = append(parts, fmt.Sprintf("%d %s", c.Runs, plural("run", c.Runs)), "started "+formatRelativeTime(c.StartedAt))
	switch {
	case c.Kept:
		parts = append(parts, "kept")
	case c.Ended:
		parts = append(parts, "ended")
	}
	return strings.Join(parts, " · ")
}

// completeValues offers values recorded on runs, such as branches.
func (a *app) completeValues(col store.RunValueColumn) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		st := a.completionStore()
		if st == nil {
			return nil, noFiles
		}
		values, err := st.RunValues(col, toComplete, completionLimit)
		if err != nil {
			return nil, noFiles
		}
		return values, noFiles
	}
}

func (a *app) completeNotes(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	st := a.completionStore()
	if st == nil || !isDigits(toComplete) {
		return nil, noFiles
	}
	notes, err := st.NoteChoices(toComplete, completionLimit)
	if err != nil {
		return nil, noFiles
	}
	out := make([]cobra.Completion, 0, len(notes))
	for _, n := range notes {
		out = append(out, cobra.CompletionWithDesc(strconv.FormatInt(n.ID, 10), shortText(n.Text, 70)))
	}
	return out, noFiles
}

// completeDurations offers common durations for --since and --older-than.
func completeDurations(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return []cobra.Completion{
		cobra.CompletionWithDesc("1h", "the last hour"),
		cobra.CompletionWithDesc("24h", "the last day"),
		cobra.CompletionWithDesc("7d", "the last week"),
		cobra.CompletionWithDesc("30d", "the last 30 days"),
		cobra.CompletionWithDesc("90d", "the last 90 days"),
	}, noFiles
}

// completeVersions offers the releases in the built-in changelog, newest
// first. It reads nothing from the network.
func completeVersions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	releases, err := changelog.Parse(carrier.Changelog)
	if err != nil {
		return nil, noFiles
	}
	out := make([]cobra.Completion, 0, len(releases))
	for _, r := range releases {
		out = append(out, cobra.CompletionWithDesc(r.Version, r.Date))
	}
	return out, noFiles
}

// shortText puts text on one line and cuts it to at most n characters.
func shortText(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return text
}
