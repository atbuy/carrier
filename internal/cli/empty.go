package cli

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

// firstRunHint tells a new user how to record something.
const firstRunHint = `no runs recorded yet; try "carrier run echo hello" or "carrier shell"`

const noSessionsHint = `no shell sessions yet; start one with "carrier shell"`

var errNoRuns = errors.New(firstRunHint)

// getRun looks up a run by ID, reporting a missing run in plain words.
func (a *app) getRun(id int64) (*store.Run, error) {
	r, err := a.st.GetRun(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("run %d not found", id)
	}
	return r, err
}

// latestRun returns the newest run, or errNoRuns when nothing is recorded.
func (a *app) latestRun() (*store.Run, error) {
	r, err := a.st.Latest()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoRuns
	}
	return r, err
}

func (a *app) hasRuns() bool {
	_, err := a.st.Latest()
	return err == nil
}

// printDeprecation writes an orange deprecation warning to stderr.
func printDeprecation(cmd *cobra.Command, msg string) {
	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintln(w, newTheme(w).Deprecated.Render("carrier: "+msg))
}

// printHint writes a muted hint to stderr, so empty lists still explain
// themselves without adding lines to piped or JSON output.
func printHint(cmd *cobra.Command, msg string) {
	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintln(w, newTheme(w).Muted.Render("carrier: "+msg))
}

// printEmptyRunsHint explains an empty run list: the first-run hint when
// nothing is recorded at all, otherwise filtered (e.g. "no failed runs").
func (a *app) printEmptyRunsHint(cmd *cobra.Command, filtered string) {
	if !a.hasRuns() {
		printHint(cmd, firstRunHint)
		return
	}
	printHint(cmd, filtered)
}
