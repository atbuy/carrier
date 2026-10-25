package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

// timelineOptions choose what a session timeline shows.
type timelineOptions struct {
	failedOnly bool // only failed runs (notes are always shown)
	allOutput  bool // the last lines of every run, not only failed ones
	lines      int  // output lines per run
}

// timelineEvent is a run or a note, in the order they happened.
type timelineEvent struct {
	at      time.Time
	run     *store.Run
	excerpt []string
	note    *store.Note
}

// sessionTimeline is everything "session show" and "session export" need.
type sessionTimeline struct {
	session store.Session
	outcome *store.Note
	runs    int
	failed  int
	notes   int
	events  []timelineEvent
	// branches are the Git branches linked to the session.
	branches []store.BranchLink
}

func (a *app) loadTimeline(sess *store.Session, opts timelineOptions) (*sessionTimeline, error) {
	runs, err := a.st.SessionRuns(sess.ID)
	if err != nil {
		return nil, err
	}
	notes, err := a.st.ListNotes(store.NoteFilter{SessionIDs: []int64{sess.ID}}, 100000)
	if err != nil {
		return nil, err
	}
	outcome, err := a.st.SessionOutcome(sess.ID)
	if err != nil {
		return nil, err
	}
	branches, err := a.st.SessionBranches(sess.ID)
	if err != nil {
		return nil, err
	}
	tl := &sessionTimeline{session: *sess, outcome: outcome, runs: len(runs), notes: len(notes), branches: branches}
	for i := range runs {
		r := &runs[i]
		if r.Status == store.StatusFailed {
			tl.failed++
		}
		if opts.failedOnly && r.Status != store.StatusFailed {
			continue
		}
		tl.events = append(tl.events, timelineEvent{at: r.StartedAt, run: r, excerpt: runExcerpt(r, opts)})
	}
	for i := range notes {
		tl.events = append(tl.events, timelineEvent{at: notes[i].CreatedAt, note: &notes[i]})
	}
	sort.SliceStable(tl.events, func(i, j int) bool { return tl.events[i].at.Before(tl.events[j].at) })
	return tl, nil
}

// runExcerpt returns the output lines shown under a run: the error lines of a
// failed run, or with allOutput the last lines of any run.
func runExcerpt(r *store.Run, opts timelineOptions) []string {
	if opts.lines <= 0 || (!opts.allOutput && r.Status != store.StatusFailed) {
		return nil
	}
	output := displayOutput(store.RunOutputTail(r))
	if strings.TrimSpace(output) == "" {
		return nil
	}
	if !opts.allOutput {
		return store.ErrorLines(output, opts.lines)
	}
	var lines []string
	all := strings.Split(output, "\n")
	for i := len(all) - 1; i >= 0 && len(lines) < opts.lines; i-- {
		if line := strings.TrimRight(all[i], " \t"); strings.TrimSpace(line) != "" {
			lines = append([]string{line}, lines...)
		}
	}
	return lines
}

// displayOutput cleans recorded output for display as plain lines: escape
// sequences (colors, terminal queries) are removed, and a line redrawn with
// carriage returns (progress bars, spinners) keeps only what was drawn last.
func displayOutput(s string) string {
	lines := strings.Split(ansi.Strip(stripVTResponses(s)), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if j := strings.LastIndex(line, "\r"); j >= 0 {
			line = line[j+1:]
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// sessionRef resolves the session a session command works on: the argument,
// else the session of this recorded shell, else the one pinned here.
func (a *app) sessionRef(cmd *cobra.Command, args []string) (*store.Session, error) {
	switch {
	case len(args) == 1:
		return resolveSession(cmd, a.st, args[0])
	case currentSessionID() != 0:
		return a.st.GetSession(currentSessionID())
	}
	if sess := a.pinnedSession(cmd); sess != nil {
		return sess, nil
	}
	return nil, errors.New("give a session ID or label, or run this inside a recorded shell")
}

func (a *app) sessionShowCmd() *cobra.Command {
	var opts timelineOptions
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show [<id-or-label>]",
		Short: "replay a session as a timeline of commands and notes",
		Long: `Show a session as a timeline: its commands with status and duration, the
error lines of failed commands, the notes taken along the way, and the outcome
recorded with "carrier session end --note".

Without an argument it shows the session of this recorded shell, or the one
pinned for this directory. Use "carrier show <run>" for a command's full output.

  carrier session show BUG-1234
  carrier session show --failed       # only failed commands
  carrier session show --output --lines 5  # the last 5 lines of every command`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := a.sessionRef(cmd, args)
			if err != nil {
				return err
			}
			tl, err := a.loadTimeline(sess, opts)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd, tl.view())
			}
			printTimeline(cmd.OutOrStdout(), tl)
			return nil
		},
	}
	addTimelineFlags(cmd, &opts)
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func addTimelineFlags(cmd *cobra.Command, opts *timelineOptions) {
	cmd.Flags().BoolVar(&opts.failedOnly, "failed", false, "only failed commands")
	cmd.Flags().BoolVar(&opts.allOutput, "output", false, "show the last lines of every command, not only failed ones")
	cmd.Flags().IntVar(&opts.lines, "lines", 3, "output lines per command (0 for none)")
}

func printTimeline(out io.Writer, tl *sessionTimeline) {
	t := newTheme(out)
	sess := tl.session
	title := t.Accent.Bold(true).Render(fmt.Sprintf("session %d", sess.ID))
	if sess.Label != "" {
		title += t.Muted.Render(" · ") + t.Label.Render(sess.Label)
	}
	_, _ = fmt.Fprintln(out, title)
	_, _ = fmt.Fprintln(out, t.Muted.Render(timelineSummary(tl, collapseHome)))
	if len(tl.branches) > 0 {
		names := make([]string, len(tl.branches))
		for i, b := range tl.branches {
			names[i] = t.Label.Render(b.Branch) + t.Muted.Render(" ("+filepath.Base(b.GitRoot)+")")
		}
		_, _ = fmt.Fprintln(out, t.Muted.Render("Branches: ")+strings.Join(names, t.Muted.Render(", ")))
	}
	if tl.outcome != nil {
		_, _ = fmt.Fprintln(out, "\n"+t.Bold.Render("Outcome: ")+tl.outcome.Text)
	}
	if len(tl.events) == 0 {
		_, _ = fmt.Fprintln(out, "\n"+t.Muted.Render("nothing recorded"))
		return
	}
	_, _ = fmt.Fprintln(out)
	ctx := timelineContext{cwd: firstRunCWD(tl), branch: firstRunBranch(tl)}
	day := ""
	multiDay := !sameDay(sess.StartedAt, timelineEnd(tl))
	for _, ev := range tl.events {
		if d := ev.at.Local().Format("Mon Jan 2"); multiDay && d != day {
			day = d
			_, _ = fmt.Fprintln(out, t.Bold.Render(d))
		}
		clock := t.Muted.Render(ev.at.Local().Format("15:04"))
		if ev.note != nil {
			_, _ = fmt.Fprintf(out, "%s  %s  %s\n", clock, t.Label.Render("✎"), noteText(*ev.note))
			continue
		}
		r := ev.run
		if changed := ctx.update(r); changed != "" {
			_, _ = fmt.Fprintf(out, "       %s\n", t.Muted.Render(changed))
		}
		exit := ""
		if r.Status == store.StatusFailed && r.ExitCode != nil {
			exit = t.Danger.Render(fmt.Sprintf("exit %d", *r.ExitCode)) + "  "
		}
		_, _ = fmt.Fprintf(out, "%s  %s  %s  %s%s\n", clock, t.statusStyle(r.Status).Render(statusGlyph(r.Status)),
			t.Command.Render(displayCommand(r)), exit, t.Muted.Render(formatDuration(r.DurationMS)))
		for _, line := range ev.excerpt {
			_, _ = fmt.Fprintf(out, "          %s %s\n", t.Muted.Render("│"), line)
		}
	}
}

// timelineContext tracks the directory and Git branch, so the timeline only
// mentions them when they change.
type timelineContext struct{ cwd, branch string }

func (c *timelineContext) update(r *store.Run) string {
	if r.CWD == c.cwd && r.GitBranch == c.branch {
		return ""
	}
	c.cwd, c.branch = r.CWD, r.GitBranch
	return placeText(collapseHome(r.CWD), r.GitBranch)
}

func placeText(cwd, branch string) string {
	if branch == "" {
		return cwd
	}
	return cwd + " on " + branch
}

func firstRunCWD(tl *sessionTimeline) string {
	for _, ev := range tl.events {
		if ev.run != nil {
			return ev.run.CWD
		}
	}
	return ""
}

func firstRunBranch(tl *sessionTimeline) string {
	for _, ev := range tl.events {
		if ev.run != nil {
			return ev.run.GitBranch
		}
	}
	return ""
}

func timelineEnd(tl *sessionTimeline) time.Time {
	if tl.session.EndedAt != nil {
		return *tl.session.EndedAt
	}
	if n := len(tl.events); n > 0 {
		return tl.events[n-1].at
	}
	return tl.session.StartedAt
}

func sameDay(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// timelineSummary is the line under the title, e.g. "Oct 21 14:02 → 15:22
// (1h 20m) · 14 commands, 3 failed · 2 notes · ~/src/api on main".
func timelineSummary(tl *sessionTimeline, path func(string) string) string {
	sess := tl.session
	start := sess.StartedAt.Local()
	span := start.Format("Jan 2 15:04") + " → "
	if sess.EndedAt == nil {
		span += "now, active"
	} else {
		end := sess.EndedAt.Local()
		if sameDay(start, end) {
			span += end.Format("15:04")
		} else {
			span += end.Format("Jan 2 15:04")
		}
		span += " (" + formatSpan(end.Sub(start)) + ")"
	}
	parts := []string{span, fmt.Sprintf("%d %s", tl.runs, plural("command", tl.runs))}
	if tl.failed > 0 {
		parts[1] += fmt.Sprintf(", %d failed", tl.failed)
	}
	if tl.notes > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", tl.notes, plural("note", tl.notes)))
	}
	if cwd := firstRunCWD(tl); cwd != "" {
		parts = append(parts, placeText(path(cwd), firstRunBranch(tl)))
	}
	return strings.Join(parts, " · ")
}

// formatSpan renders a duration for people, e.g. "1h 20m", "12m", "45s".
func formatSpan(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// sessionShowView is the JSON form of a session timeline.
type sessionShowView struct {
	sessionView
	Outcome     *noteView          `json:"outcome,omitempty"`
	RunCount    int                `json:"run_count"`
	FailedCount int                `json:"failed_count"`
	NoteCount   int                `json:"note_count"`
	Timeline    []timelineItemView `json:"timeline"`
}

type timelineItemView struct {
	Type    string    `json:"type"` // "run" or "note"
	Run     *runView  `json:"run,omitempty"`
	Excerpt []string  `json:"excerpt,omitempty"`
	Note    *noteView `json:"note,omitempty"`
}

func (tl *sessionTimeline) view() sessionShowView {
	v := sessionShowView{
		sessionView: sessionViewFromStore(tl.session),
		RunCount:    tl.runs, FailedCount: tl.failed, NoteCount: tl.notes,
		Timeline: make([]timelineItemView, 0, len(tl.events)),
	}
	if tl.outcome != nil {
		n := noteViewFromStore(*tl.outcome)
		v.Outcome = &n
	}
	for _, ev := range tl.events {
		if ev.note != nil {
			n := noteViewFromStore(*ev.note)
			v.Timeline = append(v.Timeline, timelineItemView{Type: "note", Note: &n})
			continue
		}
		r := runViewFromStore(ev.run, false)
		v.Timeline = append(v.Timeline, timelineItemView{Type: "run", Run: &r, Excerpt: ev.excerpt})
	}
	return v
}

func (a *app) sessionExportCmd() *cobra.Command {
	var opts timelineOptions
	var format, outPath string
	cmd := &cobra.Command{
		Use:   "export [<id-or-label>]",
		Short: "write a session as Markdown, to paste into a PR or ticket",
		Long: `Write a session's timeline as Markdown, ready to paste into a pull request,
a ticket, or an incident write-up: the outcome, a summary, and each command
and note in order, with the error lines of failed commands.

Output comes from the recorded logs, which are redacted when they are
written. Captured environments are never included, and paths in your home
directory are shown with ~. Read the export before sharing it anyway.

  carrier session export BUG-1234 > bug-1234.md
  carrier session export -o notes.md --failed
  carrier session export --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "markdown" && format != "json" {
				return fmt.Errorf("unknown format %q; use markdown or json", format)
			}
			sess, err := a.sessionRef(cmd, args)
			if err != nil {
				return err
			}
			tl, err := a.loadTimeline(sess, opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if outPath != "" {
				f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				out = f
			}
			if format == "json" {
				view := tl.view()
				for _, item := range view.Timeline {
					if item.Run != nil {
						item.Run.CWD = collapseHome(item.Run.CWD)
						item.Run.GitRoot = collapseHome(item.Run.GitRoot)
						item.Run.StdoutPath, item.Run.StderrPath, item.Run.TerminalOutputPath = "", "", ""
					}
					for i := range item.Excerpt {
						item.Excerpt[i] = shortenHome(item.Excerpt[i])
					}
				}
				encoderCmd := &cobra.Command{}
				encoderCmd.SetOut(out)
				if err := writeJSON(encoderCmd, view); err != nil {
					return err
				}
			} else {
				writeTimelineMarkdown(out, tl)
			}
			if outPath != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "carrier: wrote session %s to %s\n", sessionName(sess), outPath)
			}
			return nil
		},
	}
	addTimelineFlags(cmd, &opts)
	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "output format: markdown or json")
	cmd.Flags().StringVarP(&outPath, "output-file", "o", "", "write to a file instead of stdout")
	return cmd
}

// shortenHome replaces the home directory with ~ wherever it appears.
func shortenHome(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return s
	}
	return strings.ReplaceAll(s, home, "~")
}

func writeTimelineMarkdown(out io.Writer, tl *sessionTimeline) {
	sess := tl.session
	title := fmt.Sprintf("Session %d", sess.ID)
	if sess.Label != "" {
		title = sess.Label
	}
	_, _ = fmt.Fprintf(out, "# %s\n\n", title)
	if tl.outcome != nil {
		_, _ = fmt.Fprintf(out, "**Outcome:** %s\n\n", shortenHome(tl.outcome.Text))
	}
	_, _ = fmt.Fprintf(out, "%s\n", timelineSummary(tl, func(p string) string { return mdCode(shortenHome(p)) }))
	if len(tl.events) == 0 {
		_, _ = fmt.Fprint(out, "\nNothing was recorded.\n")
		return
	}
	_, _ = fmt.Fprint(out, "\n## Timeline\n\n")
	ctx := timelineContext{cwd: firstRunCWD(tl), branch: firstRunBranch(tl)}
	multiDay := !sameDay(sess.StartedAt, timelineEnd(tl))
	for _, ev := range tl.events {
		layout := "15:04"
		if multiDay {
			layout = "Jan 2 15:04"
		}
		when := "**" + ev.at.Local().Format(layout) + "**"
		if ev.note != nil {
			kind := "Note"
			if ev.note.Kind == store.NoteOutcome {
				kind = "Outcome"
			}
			_, _ = fmt.Fprintf(out, "- %s _%s:_ %s\n", when, kind, shortenHome(ev.note.Text))
			continue
		}
		r := ev.run
		place := ""
		if ctx.update(r) != "" {
			place = "in " + mdCode(shortenHome(r.CWD))
			if r.GitBranch != "" {
				place += " on " + mdCode(r.GitBranch)
			}
			place += ": "
		}
		result := formatDuration(r.DurationMS)
		switch {
		case r.Status == store.StatusFailed && r.ExitCode != nil:
			result = fmt.Sprintf("failed, exit %d, %s", *r.ExitCode, result)
		case r.Status != store.StatusSuccess:
			result = r.Status + ", " + result
		}
		_, _ = fmt.Fprintf(out, "- %s %s%s (%s)\n", when, place, mdCode(displayCommand(r)), strings.TrimSuffix(result, ", "))
		if len(ev.excerpt) > 0 {
			lines := make([]string, len(ev.excerpt))
			for i, line := range ev.excerpt {
				lines[i] = shortenHome(line)
			}
			fence := mdFence(lines)
			_, _ = fmt.Fprintf(out, "\n  %stext\n", fence)
			for _, line := range lines {
				_, _ = fmt.Fprintf(out, "  %s\n", line)
			}
			_, _ = fmt.Fprintf(out, "  %s\n\n", fence)
		}
	}
}

// mdCode formats s as inline Markdown code, with a longer backtick run when s
// holds backticks itself.
func mdCode(s string) string {
	ticks := "`"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return ticks + s + ticks
}

// mdFence returns a code fence longer than any backtick run in lines.
func mdFence(lines []string) string {
	fence := "```"
	for _, line := range lines {
		for strings.Contains(line, fence) {
			fence += "`"
		}
	}
	return fence
}

func (a *app) sessionEndCmd() *cobra.Command {
	var note string
	var unpin bool
	cmd := &cobra.Command{
		Use:   "end [<id-or-label>]",
		Short: "end a session and record what it found",
		Long: `End a session, optionally recording its outcome: what you found or decided.
"carrier session show" and "carrier session export" put the outcome first.

Inside a recorded shell, ending its session switches the shell to a new
automatic session, so the terminal keeps recording without the ended session.
Attaching to the session later reopens it.

If the session is pinned for this directory, new terminals here would still
join it; --unpin removes the pin.

  carrier session end --note "pool size was 0 after the config change"
  carrier session end BUG-1234 --note "fixed in #42" --unpin`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := a.sessionRef(cmd, args)
			if err != nil {
				return err
			}
			if note = strings.TrimSpace(note); note != "" {
				id := sess.ID
				if _, err := a.st.CreateNote(store.CreateNote{SessionID: &id, Text: note, Kind: store.NoteOutcome, CreatedAt: time.Now()}); err != nil {
					return err
				}
			}
			out := cmd.OutOrStdout()
			statePath := recordedShellState()
			inShell := statePath != "" && currentSessionID() == sess.ID
			if inShell {
				a.dropShellRun(isSessionCommand)
			}
			a.leaveSession(sess.ID)
			_, _ = fmt.Fprintf(out, "carrier: ended session %s\n", sessionName(sess))
			if inShell {
				if err := a.moveShellToNewSession(cmd, statePath); err != nil {
					return err
				}
			}
			a.handleEndedPin(cmd, sess, unpin)
			return nil
		},
	}
	cmd.Flags().StringVarP(&note, "note", "m", "", "record the session's outcome")
	cmd.Flags().BoolVar(&unpin, "unpin", false, "remove the .carrier-session pin for this session")
	return cmd
}

// moveShellToNewSession makes the recorded shell record into a new automatic
// session, after its session ended.
func (a *app) moveShellToNewSession(cmd *cobra.Command, statePath string) error {
	id, err := a.st.CreateSession(store.CreateSession{StartedAt: time.Now(), Auto: true})
	if err != nil {
		return err
	}
	sf := &carriershell.StateFile{Path: statePath}
	state := sf.Read()
	if err := sf.Write(carriershell.State{CurrentID: state.CurrentID, CurrentLog: state.CurrentLog, SessionID: id}); err != nil {
		return err
	}
	oldHooks := hooksIgnoreSessionInfo(statePath)
	if err := carriershell.WriteSessionInfo(statePath, id, ""); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: this shell now records into session %d\n", id)
	if oldHooks {
		printOldHooksHint(cmd, state.SessionID)
	}
	return nil
}

// handleEndedPin removes, or points out, a pin for this directory that names
// the ended session, since new terminals here would reopen it.
func (a *app) handleEndedPin(cmd *cobra.Command, sess *store.Session, unpin bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path, ref, err := findPin(cwd)
	if path == "" || err != nil {
		if unpin {
			printHint(cmd, "no "+pinFileName+" here to remove")
		}
		return
	}
	pinned, err := resolveSession(nil, a.st, ref)
	if err != nil || pinned.ID != sess.ID {
		if unpin {
			printHint(cmd, fmt.Sprintf("%s pins another session; left as is", collapseHome(path)))
		}
		return
	}
	if !unpin {
		printHint(cmd, fmt.Sprintf("new terminals in %s still join this session; end with --unpin or run \"carrier session unpin\" there",
			collapseHome(filepath.Dir(path))))
		return
	}
	if err := os.Remove(path); err != nil {
		printHint(cmd, "could not remove "+collapseHome(path)+": "+err.Error())
		return
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "carrier: removed %s\n", collapseHome(path))
}
