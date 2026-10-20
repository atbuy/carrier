package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

// relativeRunRef marks a "-N" run reference after rewriteRunRefArgs has
// hidden it from flag parsing. "-1" is the latest run, "-2" the one before.
const relativeRunRef = "\x1frel:"

var relativeRunArg = regexp.MustCompile(`^-[0-9]+$`)

// runRefCommands take run references as positional arguments; noteRefCommands
// take one only as the value of -r/--run.
var (
	runRefCommands  = map[string]bool{"show": true, "tail": true, "export": true, "rerun": true, "diff": true, "label": true, "similar": true}
	noteRefCommands = map[string]bool{"note": true, "notes": true}
	// intValueFlags take a number, so a following "-N" is not a run reference.
	intValueFlags = map[string]bool{"--lines": true, "--limit": true, "-l": true}
)

// rewriteRunRefArgs hides relative run references like "-2" from flag
// parsing, which would otherwise read them as unknown shorthand flags. Only
// commands that take run references are touched; "carrier run head -1 f"
// keeps its arguments.
func rewriteRunRefArgs(root *cobra.Command, args []string) []string {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == root {
		return args
	}
	positional, valueOnly := runRefCommands[cmd.Name()], noteRefCommands[cmd.Name()]
	if !positional && !valueOnly {
		return args
	}
	out := make([]string, len(args))
	copy(out, args)
	for i, arg := range out {
		prev := ""
		if i > 0 {
			prev = args[i-1]
		}
		switch {
		case valueOnly && (prev == "-r" || prev == "--run") && relativeRunArg.MatchString(arg):
			out[i] = relativeRunRef + arg[1:]
		case valueOnly && strings.HasPrefix(arg, "--run=-") && relativeRunArg.MatchString(arg[len("--run="):]):
			out[i] = "--run=" + relativeRunRef + arg[len("--run=-"):]
		case positional && !intValueFlags[prev] && relativeRunArg.MatchString(arg):
			out[i] = relativeRunRef + arg[1:]
		}
	}
	return out
}

// displayRef shows a reference as the user typed it.
func displayRef(ref string) string {
	if n, ok := strings.CutPrefix(ref, relativeRunRef); ok {
		return "-" + n
	}
	return ref
}

// resolveRun finds the run a reference names:
//
//	42        run 42, or a run whose label matches "42" if there is no run 42
//	last, -1  the latest run; -2 is the one before it
//	deploy    the newest run whose label matches
func (a *app) resolveRun(cmd *cobra.Command, ref string) (*store.Run, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("run reference must not be empty")
	}
	if ref == "last" {
		return a.latestRun()
	}
	rel, isRel := strings.CutPrefix(ref, relativeRunRef)
	if !isRel && relativeRunArg.MatchString(ref) {
		// "-N" reaches here when given after "--".
		rel, isRel = ref[1:], true
	}
	if isRel {
		n, err := strconv.Atoi(rel)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid run reference %q; -1 is the latest run", displayRef(ref))
		}
		r, err := a.st.NthLatestRun(n)
		if errors.Is(err, sql.ErrNoRows) {
			if !a.hasRuns() {
				return nil, errNoRuns
			}
			return nil, fmt.Errorf("run %s not found; there are fewer than %d runs", displayRef(ref), n)
		}
		return r, err
	}

	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		r, err := a.st.GetRun(id)
		if err == nil {
			return r, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		match, matchErr := a.findRunByLabel(ref)
		if matchErr != nil {
			if errors.Is(matchErr, errNoLabelMatch) {
				return nil, fmt.Errorf("run %d not found", id)
			}
			return nil, matchErr
		}
		printHint(cmd, fmt.Sprintf("no run %d; using run %d labeled %q", id, match.ID, match.Label))
		return match, nil
	}

	r, err := a.findRunByLabel(ref)
	if errors.Is(err, errNoLabelMatch) {
		return nil, fmt.Errorf("no run labeled %q", ref)
	}
	return r, err
}

func (a *app) findRunByLabel(query string) (*store.Run, error) {
	runs, err := a.st.ListLabeledRuns()
	if err != nil {
		return nil, err
	}
	cands := make([]labelCandidate, len(runs))
	for i, r := range runs {
		cands[i] = labelCandidate{ID: r.ID, Label: r.Label}
	}
	match, err := pickLabel("run", query, cands)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		if runs[i].ID == match.ID {
			return &runs[i], nil
		}
	}
	return nil, errNoLabelMatch
}

// resolveSession finds a session by ID or label. A number that is not a
// session ID is matched against labels too, so "carrier attach 1234" finds a
// session labeled "BUG-1234: fix login".
func resolveSession(cmd *cobra.Command, st *store.Store, ref string) (*store.Session, error) {
	ref = strings.TrimSpace(ref)
	id, idErr := strconv.ParseInt(ref, 10, 64)
	if idErr == nil {
		sess, err := st.GetSession(id)
		if err == nil {
			return sess, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	sessions, err := st.ListLabeledSessions()
	if err != nil {
		return nil, err
	}
	cands := make([]labelCandidate, len(sessions))
	for i, s := range sessions {
		cands[i] = labelCandidate{ID: s.ID, Label: s.Label}
	}
	match, err := pickLabel("session", ref, cands)
	if errors.Is(err, errNoLabelMatch) {
		if idErr == nil {
			return nil, fmt.Errorf("session %d not found; list sessions with \"carrier session list\"", id)
		}
		return nil, fmt.Errorf("no session with label %q; list sessions with \"carrier session list\"", ref)
	}
	if err != nil {
		return nil, err
	}
	if idErr == nil && cmd != nil {
		printHint(cmd, fmt.Sprintf("no session %d; using session %d %q", id, match.ID, match.Label))
	}
	for i := range sessions {
		if sessions[i].ID == match.ID {
			return &sessions[i], nil
		}
	}
	return nil, errNoLabelMatch
}

type labelCandidate struct {
	ID    int64
	Label string
}

var errNoLabelMatch = errors.New("no label matches")

// pickLabel matches query against candidate labels (newest first), trying in
// order, case-insensitively:
//
//  1. the whole label
//  2. whole words: "1234" or "bug-1234" match "BUG-1234: fix login", but
//     "123" does not
//  3. a substring
//
// The first step with matches decides. If they all share one label, the
// newest wins; if they differ ("1234" matching both "BUG-1234" and
// "OPS-1234"), the reference is ambiguous.
func pickLabel(kind, query string, cands []labelCandidate) (labelCandidate, error) {
	q := strings.TrimSpace(query)
	qWords := labelWords(q)
	steps := []func(label string) bool{
		func(label string) bool { return strings.EqualFold(strings.TrimSpace(label), q) },
		func(label string) bool { return len(qWords) > 0 && containsWords(labelWords(label), qWords) },
		func(label string) bool { return q != "" && containsFold(label, q) },
	}
	for _, matches := range steps {
		var found []labelCandidate
		labels := map[string]bool{}
		for _, c := range cands {
			if matches(c.Label) {
				found = append(found, c)
				labels[strings.ToLower(strings.TrimSpace(c.Label))] = true
			}
		}
		if len(found) == 0 {
			continue
		}
		if len(labels) == 1 {
			return found[0], nil
		}
		return labelCandidate{}, ambiguousLabelError(kind, query, found)
	}
	return labelCandidate{}, errNoLabelMatch
}

func ambiguousLabelError(kind, query string, found []labelCandidate) error {
	seen := map[string]bool{}
	var parts []string
	for _, c := range found {
		key := strings.ToLower(strings.TrimSpace(c.Label))
		if seen[key] {
			continue
		}
		seen[key] = true
		parts = append(parts, fmt.Sprintf("%s %d %q", kind, c.ID, c.Label))
		if len(parts) == 5 {
			break
		}
	}
	return fmt.Errorf("%q matches several %s labels: %s; use the %s ID or more of the label",
		query, kind, strings.Join(parts, ", "), kind)
}

// labelWords splits a label into lowercase words of letters and digits.
func labelWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// containsWords reports whether want appears in words as a contiguous run.
func containsWords(words, want []string) bool {
	for i := 0; i+len(want) <= len(words); i++ {
		match := true
		for j := range want {
			if words[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
