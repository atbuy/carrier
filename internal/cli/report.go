package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/duration"
	"github.com/atbuy/carrier/internal/store"
)

// reportNow is the time a report is made at; tests fix it.
var reportNow = time.Now

// reportCommandsShown and reportFailingShown cap the lists under each place.
const (
	reportCommandsShown = 5
	reportFailingShown  = 5
)

func (a *app) reportCmd() *cobra.Command {
	var (
		since      string
		repo       bool
		jsonOutput bool
		markdown   bool
	)
	cmd := &cobra.Command{
		Use:     "report [today|yesterday|week|<date>]",
		Aliases: []string{"rp"},
		Short:   "summarize what you worked on",
		Long: `Summarize what you worked on: the repositories and directories you ran
commands in, the sessions there with their notes and outcomes, the commands
you ran most, and the ones that still fail.

  carrier report                # today
  carrier report yesterday
  carrier report week           # the last 7 days
  carrier report 2026-10-20     # one day
  carrier report --since 3h
  carrier report --repo         # only the repository you are in
  carrier report --markdown     # to paste into a standup or a pull request`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput && markdown {
				return errors.New("choose one of --json and --markdown")
			}
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			period, err := parseReportPeriod(arg, since, reportNow())
			if err != nil {
				return err
			}
			f := store.HistoryFilter{Since: period.From, Until: period.To}
			if repo {
				if f.RepoRoots, err = currentRepoRoots(); err != nil {
					return err
				}
			}
			rep, err := a.buildReport(period, f)
			if err != nil {
				return err
			}
			switch {
			case jsonOutput:
				return writeJSON(cmd, rep.view())
			case markdown:
				rep.writeMarkdown(cmd.OutOrStdout())
			default:
				rep.writeText(cmd.OutOrStdout())
			}
			if len(rep.runs) == 0 && !a.hasRuns() {
				printHint(cmd, firstRunHint)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "report on this long until now, such as 3h or 2d, instead of a day")
	cmd.Flags().BoolVar(&repo, "repo", false, "only the Git repository you are in")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "output Markdown")
	return cmd
}

// reportPeriod is the time a report covers, from From up to, not including,
// To.
type reportPeriod struct {
	// Name says the period in a sentence: "today", "the last 7 days".
	Name     string
	From, To time.Time
	// days is true for periods of whole days in the past, whose times are
	// shown with the day.
	days int
}

func parseReportPeriod(arg, since string, now time.Time) (reportPeriod, error) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if since != "" {
		if arg != "" {
			return reportPeriod{}, errors.New("give a period or --since, not both")
		}
		d, err := duration.Parse(since)
		if err != nil {
			return reportPeriod{}, fmt.Errorf("invalid --since duration %q: %w", since, err)
		}
		from := now.Add(-d)
		return reportPeriod{Name: "the last " + since, From: from, To: now, days: daysBetween(from, now)}, nil
	}
	switch arg {
	case "", "today":
		return reportPeriod{Name: "today", From: midnight, To: now, days: 1}, nil
	case "yesterday":
		return reportPeriod{Name: "yesterday", From: midnight.AddDate(0, 0, -1), To: midnight, days: 1}, nil
	case "week":
		return reportPeriod{Name: "the last 7 days", From: midnight.AddDate(0, 0, -6), To: now, days: 7}, nil
	}
	day, err := time.ParseInLocation("2006-01-02", arg, now.Location())
	if err != nil {
		return reportPeriod{}, fmt.Errorf("unknown period %q; use today, yesterday, week, or a date such as %s", arg, midnight.AddDate(0, 0, -3).Format("2006-01-02"))
	}
	return reportPeriod{Name: day.Format("Monday 2 January 2006"), From: day, To: day.AddDate(0, 0, 1), days: 1}, nil
}

// daysBetween counts the calendar days that from up to to touches.
func daysBetween(from, to time.Time) int {
	n := 1
	for d := from; ; n++ {
		d = d.AddDate(0, 0, 1)
		if d.Year() > to.Year() || (d.Year() == to.Year() && d.YearDay() > to.YearDay()) {
			return n
		}
	}
}

// report is what a period's runs and notes add up to.
type report struct {
	period   reportPeriod
	runs     []store.Run
	failed   int
	sessions int
	// commandMS is the time spent in commands that finished.
	commandMS int64
	places    []*reportPlace
}

// reportPlace is a repository, or a directory outside any, and what was done
// there.
type reportPlace struct {
	path     string
	repo     bool
	runs     int
	failed   int
	branches []string
	first    time.Time
	last     time.Time
	// groups are the sessions worked in here in the order they started, then
	// the runs and notes outside a session.
	groups       []*reportGroup
	commands     []reportCommand
	stillFailing []*store.Run
}

// reportGroup is a session's part of a place, or, with no session, the runs
// and notes there outside any session.
type reportGroup struct {
	session *store.Session
	runs    []*store.Run
	failed  int
	notes   []store.Note
	outcome *store.Note
}

type reportCommand struct {
	command string
	runs    int
}

func (a *app) buildReport(period reportPeriod, f store.HistoryFilter) (*report, error) {
	runs, err := a.st.ListHistory(1_000_000, f)
	if err != nil {
		return nil, err
	}
	// Oldest first, as the day went.
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
	rep := &report{period: period, runs: runs}
	places := map[string]*reportPlace{}
	groups := map[string]*reportGroup{}
	runPlace := map[int64]string{}
	var sessionIDs []int64
	seenSession := map[int64]bool{}
	placeFor := func(r *store.Run) *reportPlace {
		key, repo := runPlaceKey(r)
		p := places[key]
		if p == nil {
			p = &reportPlace{path: key, repo: repo}
			places[key] = p
			rep.places = append(rep.places, p)
		}
		return p
	}
	groupFor := func(p *reportPlace, sessionID *int64) *reportGroup {
		key := p.path + "\x00"
		if sessionID != nil {
			key += fmt.Sprint(*sessionID)
		}
		g := groups[key]
		if g == nil {
			g = &reportGroup{}
			if sessionID != nil {
				g.session = &store.Session{ID: *sessionID}
			}
			groups[key] = g
			p.groups = append(p.groups, g)
		}
		return g
	}
	for i := range runs {
		r := &runs[i]
		p := placeFor(r)
		g := groupFor(p, r.SessionID)
		g.runs = append(g.runs, r)
		if p.runs == 0 {
			p.first = r.StartedAt
		}
		p.runs++
		p.last = r.StartedAt
		runPlace[r.ID] = p.path
		if r.Status == store.StatusFailed {
			p.failed++
			g.failed++
			rep.failed++
		}
		if r.DurationMS != nil {
			rep.commandMS += *r.DurationMS
		}
		if r.GitBranch != "" && !slices.Contains(p.branches, r.GitBranch) {
			p.branches = append(p.branches, r.GitBranch)
		}
		if r.SessionID != nil && !seenSession[*r.SessionID] {
			seenSession[*r.SessionID] = true
			sessionIDs = append(sessionIDs, *r.SessionID)
		}
	}
	rep.sessions = len(sessionIDs)

	notes, err := a.st.NotesBetween(period.From, period.To)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		g := a.reportNoteGroup(n, places, placeFor, runPlace, groupFor, f)
		if g == nil {
			continue
		}
		if n.Kind == store.NoteOutcome {
			g.outcome = &n
		} else {
			g.notes = append(g.notes, n)
		}
		if g.session != nil && !seenSession[g.session.ID] {
			seenSession[g.session.ID] = true
			sessionIDs = append(sessionIDs, g.session.ID)
		}
	}

	sessions, err := a.st.GetSessionsByIDs(sessionIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range rep.places {
		for _, g := range p.groups {
			if g.session != nil {
				if sess, ok := sessions[g.session.ID]; ok {
					g.session = &sess
				}
			}
		}
		// Runs outside a session come after the sessions.
		sort.SliceStable(p.groups, func(i, j int) bool { return p.groups[i].session != nil && p.groups[j].session == nil })
		p.commands, p.stillFailing = summarizeCommands(p.groups)
	}
	// The places with the most done first.
	sort.SliceStable(rep.places, func(i, j int) bool { return rep.places[i].runs > rep.places[j].runs })
	return rep, nil
}

// reportNoteGroup finds where a note belongs: with its run, or with its
// session where that session ran commands. A note on a run or session from
// before the period goes where that run, or the session's last run, was.
// Notes that match neither, or fall outside --repo, are left out.
func (a *app) reportNoteGroup(n store.Note, places map[string]*reportPlace, placeFor func(*store.Run) *reportPlace,
	runPlace map[int64]string, groupFor func(*reportPlace, *int64) *reportGroup, f store.HistoryFilter,
) *reportGroup {
	var run *store.Run
	if n.RunID != nil {
		if path, ok := runPlace[*n.RunID]; ok {
			r := findReportRun(places[path], *n.RunID)
			return groupFor(places[path], r.SessionID)
		}
		run, _ = a.st.GetRun(*n.RunID)
	}
	if n.SessionID != nil {
		// The place where the session ran the most commands in the period.
		var best *reportPlace
		bestRuns := 0
		for _, p := range places {
			for _, g := range p.groups {
				if g.session != nil && g.session.ID == *n.SessionID && len(g.runs) > bestRuns {
					best, bestRuns = p, len(g.runs)
				}
			}
		}
		if best != nil {
			return groupFor(best, n.SessionID)
		}
		if run == nil {
			run, _ = a.st.LatestSessionRun(*n.SessionID)
		}
	}
	if run == nil || !reportRunMatches(run, f) {
		return nil
	}
	// The place may have notes but no commands in the period.
	return groupFor(placeFor(run), n.SessionID)
}

// runPlaceKey is where a run counts in a report: its repository, or its
// directory outside any.
func runPlaceKey(r *store.Run) (key string, repo bool) {
	if r.GitRoot != "" {
		return r.GitRoot, true
	}
	return r.CWD, false
}

// reportRunMatches applies --repo to a run from outside the period, which
// places a note.
func reportRunMatches(r *store.Run, f store.HistoryFilter) bool {
	if len(f.RepoRoots) == 0 {
		return true
	}
	for _, root := range f.RepoRoots {
		if r.GitRoot == root || (r.GitRoot == "" && (r.CWD == root || strings.HasPrefix(r.CWD, root+string(filepath.Separator)))) {
			return true
		}
	}
	return false
}

func findReportRun(p *reportPlace, id int64) *store.Run {
	for _, g := range p.groups {
		for _, r := range g.runs {
			if r.ID == id {
				return r
			}
		}
	}
	return &store.Run{}
}

// summarizeCommands counts the commands run in a place, most run first, and
// finds the ones whose last run failed.
func summarizeCommands(groups []*reportGroup) ([]reportCommand, []*store.Run) {
	counts := map[string]int{}
	lastRun := map[string]*store.Run{}
	var order []string
	for _, g := range groups {
		for _, r := range g.runs {
			c := strings.Join(strings.Fields(displayCommand(r)), " ")
			if c == "" {
				continue
			}
			if counts[c] == 0 {
				order = append(order, c)
			}
			counts[c]++
			if last := lastRun[c]; last == nil || r.ID > last.ID {
				lastRun[c] = r
			}
		}
	}
	commands := make([]reportCommand, 0, len(order))
	for _, c := range order {
		commands = append(commands, reportCommand{command: c, runs: counts[c]})
	}
	sort.SliceStable(commands, func(i, j int) bool { return commands[i].runs > commands[j].runs })
	var failing []*store.Run
	for _, c := range order {
		if r := lastRun[c]; r.Status == store.StatusFailed {
			failing = append(failing, r)
		}
	}
	sort.Slice(failing, func(i, j int) bool { return failing[i].ID < failing[j].ID })
	return commands, failing
}

// title is the report's heading: "Today, Monday 26 October 2026".
func (rep *report) title() string {
	p := rep.period
	name := strings.ToUpper(p.Name[:1]) + p.Name[1:]
	switch {
	case p.Name == "today" || p.Name == "yesterday":
		return name + ", " + p.From.Format("Monday 2 January 2006")
	case p.days == 1 && !strings.HasPrefix(p.Name, "the last"):
		return name
	default:
		return name + ", from " + p.From.Format("Mon 2 Jan 15:04")
	}
}

// clock shows a time of day, with the day when the period spans days.
func (rep *report) clock(t time.Time) string {
	if rep.period.days > 1 {
		return t.Local().Format("Mon 15:04")
	}
	return t.Local().Format("15:04")
}

func (rep *report) span(first, last time.Time) string {
	if first.IsZero() {
		return ""
	}
	if rep.clock(first) == rep.clock(last) {
		return rep.clock(first)
	}
	return rep.clock(first) + "–" + rep.clock(last)
}

// summary is the line under the title: "42 runs in 3 sessions, 3 failed".
func (rep *report) summary() string {
	if len(rep.runs) == 0 {
		return "No commands recorded " + rep.period.during() + "."
	}
	line := count(len(rep.runs), "run")
	if rep.sessions > 0 {
		line += " in " + count(rep.sessions, "session")
	}
	if rep.failed > 0 {
		line += fmt.Sprintf(", %d failed", rep.failed)
	}
	if rep.commandMS >= 1000 {
		line += ", " + roughDuration(time.Duration(rep.commandMS)*time.Millisecond) + " in commands"
	}
	return line + "."
}

// during says the period after a verb: "today", "in the last 7 days".
func (p reportPeriod) during() string {
	switch {
	case p.Name == "today" || p.Name == "yesterday":
		return p.Name
	case strings.HasPrefix(p.Name, "the last"):
		return "in " + p.Name
	default:
		return "on " + p.Name
	}
}

// reportEntry is a line under a place: a session, the sessions with nothing
// to say but their runs, or the runs outside sessions.
type reportEntry struct {
	name string
	// id is the labeled session's ID, or 0.
	id int64
	// session is true for a line of one session.
	session bool
	runs    int
	failed  int
	span    string
	outcome *store.Note
	notes   []store.Note
}

// entries lists a place's sessions for reading: those with a label, notes,
// or an outcome each on their own, the rest, such as the automatic session
// of every terminal, together on one line, and the runs outside sessions
// last. A place whose runs were all outside sessions has no entries; its own
// line says it all.
func (rep *report) entries(p *reportPlace) []reportEntry {
	var out []reportEntry
	var rest reportEntry
	restSessions := 0
	var outside *reportGroup
	for _, g := range p.groups {
		switch {
		case g.session == nil:
			outside = g
		case g.session.Label != "" || len(g.notes) > 0 || g.outcome != nil:
			e := reportEntry{name: g.session.Label, id: g.session.ID, session: true, runs: len(g.runs), failed: g.failed, outcome: g.outcome, notes: g.notes}
			if e.name == "" {
				e.name, e.id = fmt.Sprintf("session %d", g.session.ID), 0
			}
			if len(g.runs) > 0 {
				e.span = rep.span(g.runs[0].StartedAt, g.runs[len(g.runs)-1].StartedAt)
			}
			out = append(out, e)
		default:
			restSessions++
			rest.runs += len(g.runs)
			rest.failed += g.failed
		}
	}
	if restSessions > 0 {
		rest.name = count(restSessions, "session") + " without a label"
		if len(out) > 0 {
			rest.name = count(restSessions, "other session")
		}
		out = append(out, rest)
	}
	if outside != nil && (len(out) > 0 || len(outside.notes) > 0) {
		out = append(out, reportEntry{name: "outside sessions", runs: len(outside.runs), failed: outside.failed, notes: outside.notes})
	}
	return out
}

// counts says a place's or a group's runs: "14 runs, 2 failed".
func runCounts(runs, failed int) string {
	s := count(runs, "run")
	if failed > 0 {
		s += fmt.Sprintf(", %d failed", failed)
	}
	return s
}

// count says n of word: "1 run", "3 runs".
func count(n int, word string) string { return fmt.Sprintf("%d %s", n, plural(word, n)) }

// roughDuration rounds d for reading: "2h 10m", "4m", "35s".
func roughDuration(d time.Duration) string {
	if s := d.Round(time.Second); s < time.Minute {
		return fmt.Sprintf("%ds", int(s.Seconds()))
	}
	m := d.Round(time.Minute)
	switch {
	case m < time.Hour:
		return fmt.Sprintf("%dm", int(m.Minutes()))
	case m%time.Hour == 0:
		return fmt.Sprintf("%dh", int(m.Hours()))
	default:
		return fmt.Sprintf("%dh %dm", int(m.Hours()), int(m.Minutes())%60)
	}
}

// repeated is the commands run more than once in a place, most run first:
// what a command run once is doing is better told by the sessions.
func (p *reportPlace) repeated() []reportCommand {
	var out []reportCommand
	for _, c := range p.commands {
		if c.runs > 1 && len(out) < reportCommandsShown {
			out = append(out, c)
		}
	}
	return out
}

func (p *reportPlace) name() string {
	if p.repo {
		return filepath.Base(p.path)
	}
	return collapseHome(p.path)
}

func (rep *report) writeText(w io.Writer) {
	t := newTheme(w)
	_, _ = fmt.Fprintln(w, t.Bold.Render(rep.title()))
	_, _ = fmt.Fprintln(w, t.Muted.Render(rep.summary()))
	for _, p := range rep.places {
		_, _ = fmt.Fprintln(w)
		head := t.Bold.Render(p.name())
		var details []string
		if p.repo {
			details = append(details, collapseHome(p.path))
		}
		if len(p.branches) > 0 {
			details = append(details, strings.Join(p.branches, ", "))
		}
		if len(details) > 0 {
			head += "  " + t.Muted.Render(strings.Join(details, " · "))
		}
		_, _ = fmt.Fprintln(w, head)
		if p.runs > 0 {
			_, _ = fmt.Fprintln(w, "  "+t.Muted.Render(rep.span(p.first, p.last)+" · ")+failedStyled(t, runCounts(p.runs, p.failed), p.failed))
		}
		for _, e := range rep.entries(p) {
			name := t.Muted.Render(e.name)
			if e.session {
				name = t.Label.Render(e.name)
			}
			if e.id != 0 {
				name += t.Muted.Render(fmt.Sprintf(" (session %d)", e.id))
			}
			var parts []string
			if e.span != "" {
				parts = append(parts, e.span)
			}
			if e.runs > 0 {
				parts = append(parts, runCounts(e.runs, e.failed))
			}
			line := "  " + t.Accent.Render("•") + " " + name
			if len(parts) > 0 {
				line += t.Muted.Render(" · ") + failedStyled(t, strings.Join(parts, " · "), e.failed)
			}
			_, _ = fmt.Fprintln(w, line)
			if e.outcome != nil {
				_, _ = fmt.Fprintln(w, "      "+t.Bold.Render("outcome: ")+oneLine(e.outcome.Text))
			}
			for _, n := range e.notes {
				_, _ = fmt.Fprintln(w, "      "+t.Muted.Render(rep.clock(n.CreatedAt)+" ")+oneLine(n.Text))
			}
		}
		if cs := p.repeated(); len(cs) > 0 {
			var parts []string
			for _, c := range cs {
				parts = append(parts, t.Command.Render(clip(c.command, 40))+t.Muted.Render(fmt.Sprintf(" ×%d", c.runs)))
			}
			_, _ = fmt.Fprintln(w, "  "+t.Muted.Render("most run: ")+strings.Join(parts, t.Muted.Render(" · ")))
		}
		for i, r := range p.stillFailing {
			if i == reportFailingShown {
				_, _ = fmt.Fprintln(w, "  "+t.Muted.Render(fmt.Sprintf("… %d more still failing", len(p.stillFailing)-i)))
				break
			}
			_, _ = fmt.Fprintf(w, "  %s %s  %s\n", t.Danger.Render("✗ still failing:"), t.Command.Render(clip(oneLine(displayCommand(r)), 50)),
				t.Muted.Render(fmt.Sprintf("run %d, %s", r.ID, rep.clock(r.StartedAt))))
		}
	}
}

func failedStyled(t theme, s string, failed int) string {
	if failed > 0 {
		return t.Danger.Render(s)
	}
	return t.Muted.Render(s)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// clip shortens s to width columns, ending it with "…" when cut.
func clip(s string, width int) string { return ansi.Truncate(s, width, "…") }

func (rep *report) writeMarkdown(w io.Writer) {
	_, _ = fmt.Fprintf(w, "## %s\n\n%s\n", rep.title(), rep.summary())
	for _, p := range rep.places {
		_, _ = fmt.Fprintf(w, "\n### %s\n\n", markdownText(p.name()))
		var details []string
		if p.repo {
			details = append(details, markdownCode(collapseHome(p.path)))
		}
		if len(p.branches) > 0 {
			var bs []string
			for _, b := range p.branches {
				bs = append(bs, markdownCode(b))
			}
			details = append(details, "on "+strings.Join(bs, ", "))
		}
		if p.runs > 0 {
			details = append(details, runCounts(p.runs, p.failed)+", "+rep.span(p.first, p.last))
		}
		if len(details) > 0 {
			_, _ = fmt.Fprintf(w, "%s\n\n", strings.Join(details, " · "))
		}
		for _, e := range rep.entries(p) {
			line := "- " + markdownText(e.name)
			if e.session {
				line = "- **" + markdownText(e.name) + "**"
			}
			if e.id != 0 {
				line += fmt.Sprintf(" (session %d)", e.id)
			}
			var parts []string
			if e.runs > 0 {
				parts = append(parts, runCounts(e.runs, e.failed))
			}
			if e.span != "" {
				parts = append(parts, e.span)
			}
			if len(parts) > 0 {
				line += ": " + strings.Join(parts, ", ")
			}
			_, _ = fmt.Fprintln(w, line)
			if e.outcome != nil {
				_, _ = fmt.Fprintf(w, "  - Outcome: %s\n", markdownText(oneLine(e.outcome.Text)))
			}
			for _, n := range e.notes {
				_, _ = fmt.Fprintf(w, "  - Note: %s\n", markdownText(oneLine(n.Text)))
			}
		}
		if cs := p.repeated(); len(cs) > 0 {
			var parts []string
			for _, c := range cs {
				parts = append(parts, markdownCode(c.command)+fmt.Sprintf(" ×%d", c.runs))
			}
			_, _ = fmt.Fprintf(w, "- Most run: %s\n", strings.Join(parts, ", "))
		}
		if len(p.stillFailing) > 0 {
			var cs []string
			for _, r := range p.stillFailing[:min(len(p.stillFailing), reportFailingShown)] {
				cs = append(cs, markdownCode(oneLine(displayCommand(r))))
			}
			if more := len(p.stillFailing) - reportFailingShown; more > 0 {
				cs = append(cs, fmt.Sprintf("%d more", more))
			}
			_, _ = fmt.Fprintf(w, "- Still failing: %s\n", strings.Join(cs, ", "))
		}
	}
}

// markdownText escapes the characters that would format text in Markdown.
func markdownText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_[]<>#|", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// markdownCode writes s as inline code, with a fence longer than any run of
// backticks in it.
func markdownCode(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if longest > 0 {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

type reportView struct {
	Period            string            `json:"period"`
	From              time.Time         `json:"from"`
	To                time.Time         `json:"to"`
	Runs              int               `json:"runs"`
	Failed            int               `json:"failed"`
	Sessions          int               `json:"sessions"`
	CommandDurationMS int64             `json:"command_duration_ms"`
	Places            []reportPlaceView `json:"places"`
}

type reportPlaceView struct {
	Path         string                `json:"path"`
	Repo         bool                  `json:"repo"`
	Branches     []string              `json:"branches"`
	Runs         int                   `json:"runs"`
	Failed       int                   `json:"failed"`
	First        *time.Time            `json:"first,omitempty"`
	Last         *time.Time            `json:"last,omitempty"`
	Sessions     []reportSessionView   `json:"sessions"`
	Outside      *reportOutsideView    `json:"outside_sessions,omitempty"`
	Commands     []reportCommandView   `json:"commands"`
	StillFailing []reportFailedRunView `json:"still_failing"`
}

type reportSessionView struct {
	ID      int64            `json:"id"`
	Label   string           `json:"label,omitempty"`
	Runs    int              `json:"runs"`
	Failed  int              `json:"failed"`
	First   *time.Time       `json:"first,omitempty"`
	Last    *time.Time       `json:"last,omitempty"`
	Outcome string           `json:"outcome,omitempty"`
	Notes   []reportNoteView `json:"notes"`
}

type reportOutsideView struct {
	Runs   int              `json:"runs"`
	Failed int              `json:"failed"`
	Notes  []reportNoteView `json:"notes"`
}

type reportNoteView struct {
	ID        int64     `json:"id"`
	RunID     *int64    `json:"run_id,omitempty"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type reportCommandView struct {
	Command string `json:"command"`
	Runs    int    `json:"runs"`
}

type reportFailedRunView struct {
	ID        int64     `json:"id"`
	Command   string    `json:"command"`
	StartedAt time.Time `json:"started_at"`
}

func (rep *report) view() reportView {
	v := reportView{
		Period: rep.period.Name, From: rep.period.From, To: rep.period.To,
		Runs: len(rep.runs), Failed: rep.failed, Sessions: rep.sessions, CommandDurationMS: rep.commandMS,
		Places: []reportPlaceView{},
	}
	for _, p := range rep.places {
		pv := reportPlaceView{
			Path: p.path, Repo: p.repo, Branches: append([]string{}, p.branches...), Runs: p.runs, Failed: p.failed,
			Sessions: []reportSessionView{}, Commands: []reportCommandView{}, StillFailing: []reportFailedRunView{},
		}
		if p.runs > 0 {
			pv.First, pv.Last = &p.first, &p.last
		}
		for _, g := range p.groups {
			notes := []reportNoteView{}
			for _, n := range g.notes {
				notes = append(notes, reportNoteView{ID: n.ID, RunID: n.RunID, Text: n.Text, CreatedAt: n.CreatedAt})
			}
			if g.session == nil {
				pv.Outside = &reportOutsideView{Runs: len(g.runs), Failed: g.failed, Notes: notes}
				continue
			}
			sv := reportSessionView{ID: g.session.ID, Label: g.session.Label, Runs: len(g.runs), Failed: g.failed, Notes: notes}
			if len(g.runs) > 0 {
				sv.First, sv.Last = &g.runs[0].StartedAt, &g.runs[len(g.runs)-1].StartedAt
			}
			if g.outcome != nil {
				sv.Outcome = g.outcome.Text
			}
			pv.Sessions = append(pv.Sessions, sv)
		}
		for _, c := range p.commands {
			pv.Commands = append(pv.Commands, reportCommandView{Command: c.command, Runs: c.runs})
		}
		for _, r := range p.stillFailing {
			pv.StillFailing = append(pv.StillFailing, reportFailedRunView{ID: r.ID, Command: displayCommand(r), StartedAt: r.StartedAt})
		}
		v.Places = append(v.Places, pv)
	}
	return v
}
