// Package hints builds optional one-line hints printed after a run finishes.
package hints

import (
	"fmt"
	"strings"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

// exitInterrupted is the exit code of a command stopped with Ctrl-C.
const exitInterrupted = 130

// SimilarFailure returns a hint pointing at an earlier failed run with the
// same error, or "" when hints.similar_failures is off, the run was
// interrupted, or nothing matches. The hint has no "carrier:" prefix.
func SimilarFailure(st *store.Store, cfg config.Config, runID int64, exitCode int) string {
	if !cfg.Hints.SimilarFailures || exitCode == exitInterrupted {
		return ""
	}
	matches, err := st.SimilarFailures(runID, time.Now().Add(-cfg.SimilarMinAgeDuration()), 2)
	if err != nil || len(matches) == 0 {
		return ""
	}
	best := matches[0]
	hint := fmt.Sprintf("seen before in run %d (%s): carrier show %d", best.Run.ID, strings.Join(Details(st, best), ", "), best.Run.ID)
	if len(matches) > 1 {
		hint += fmt.Sprintf("; more: carrier similar %d", runID)
	}
	return hint
}

// Details describes a similar run: when it ran, plus its session, label, and
// notes when it has them.
func Details(st *store.Store, m store.SimilarRun) []string {
	parts := []string{Ago(m.Run.StartedAt, time.Now())}
	if m.Run.SessionID != nil {
		if sess, err := st.GetSession(*m.Run.SessionID); err == nil && sess.Label != "" {
			parts = append(parts, fmt.Sprintf("session %q", sess.Label))
		}
	}
	if m.Run.Label != "" {
		parts = append(parts, fmt.Sprintf("label %q", m.Run.Label))
	}
	switch m.NoteCount {
	case 0:
	case 1:
		parts = append(parts, "1 note")
	default:
		parts = append(parts, fmt.Sprintf("%d notes", m.NoteCount))
	}
	return parts
}

// Ago formats how long before now t was, e.g. "3 weeks ago".
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, unit)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 14*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	case d < 60*24*time.Hour:
		return plural(int(d.Hours()/(24*7)), "week")
	case d < 365*24*time.Hour:
		return plural(int(d.Hours()/(24*30)), "month")
	default:
		return plural(int(d.Hours()/(24*365)), "year")
	}
}
