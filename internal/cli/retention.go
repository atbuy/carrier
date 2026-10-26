package cli

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
	"github.com/atbuy/carrier/internal/store"
)

const (
	sessionRetentionInterval = 24 * time.Hour
	sessionRetentionFilename = "session-retention.timestamp"
	tempCleanupFilename      = "temp-cleanup.timestamp"
)

// cleanTempIfDue removes, at most once a day, the temp files of recorded
// shells that were killed instead of exiting.
func (a *app) cleanTempIfDue() {
	dataDir := a.cfg.Storage.DataDir
	if !sidecarDue(dataDir, tempCleanupFilename, sessionRetentionInterval) {
		return
	}
	touchSidecar(dataDir, tempCleanupFilename)
	_, killed := carriershell.CleanTemp(time.Now())
	a.finishKilledShells(killed)
}

// finishKilledShells finishes what shells killed outright, such as with
// "kill -9", left behind: the command still running in each is marked killed,
// and its session ended. A shell that hangs up finishes these itself.
func (a *app) finishKilledShells(states []carriershell.State) {
	for _, state := range states {
		if state.CurrentID != 0 {
			if r, err := a.st.GetRun(state.CurrentID); err == nil && r.Status == store.StatusRunning {
				// When it died is not known; it last showed signs of life
				// when it wrote output. 137 is 128 + SIGKILL.
				ended := r.StartedAt
				if info, err := os.Stat(r.TerminalOutputPath); err == nil && info.ModTime().After(ended) {
					ended = info.ModTime()
				}
				_ = a.st.FinishRun(r.ID, store.StatusKilled, 137, ended)
			}
		}
		if state.SessionID != 0 {
			if sess, err := a.st.GetSession(state.SessionID); err == nil && sess.EndedAt == nil {
				_ = a.st.EndSession(sess.ID, time.Now())
			}
		}
	}
}

// expireAutoSessionsIfDue deletes expired automatic sessions at most once a
// day, when a shell starts. Errors are ignored: cleanup must never stop a
// terminal from opening.
func (a *app) expireAutoSessionsIfDue(cmd *cobra.Command) {
	dataDir := a.cfg.Storage.DataDir
	if a.cfg.AutoSessionRetentionDuration() == 0 || !sidecarDue(dataDir, sessionRetentionFilename, sessionRetentionInterval) {
		return
	}
	touchSidecar(dataDir, sessionRetentionFilename)
	_, _, _ = a.expireAutoSessions(time.Now(), false)
}

// expireAutoSessions finds automatic sessions that ended longer ago than
// shell.auto_session_retention and have no label, notes, or pin. Unless
// dryRun, it deletes them with their runs and logs. It returns the expired
// sessions and their runs.
func (a *app) expireAutoSessions(now time.Time, dryRun bool) ([]store.Session, []store.Run, error) {
	retention := a.cfg.AutoSessionRetentionDuration()
	if retention == 0 {
		return nil, nil, nil
	}
	sessions, err := a.st.ExpiredAutoSessions(now.Add(-retention))
	if err != nil {
		return nil, nil, err
	}
	var runs []store.Run
	for _, sess := range sessions {
		var sessRuns []store.Run
		if dryRun {
			sessRuns, err = a.st.SessionRuns(sess.ID)
		} else {
			sessRuns, err = a.st.DeleteSession(sess.ID)
		}
		if err != nil {
			return nil, nil, err
		}
		if !dryRun {
			for _, r := range sessRuns {
				removeIfSet(r.StdoutPath)
				removeIfSet(r.StderrPath)
				removeIfSet(r.TerminalOutputPath)
			}
		}
		runs = append(runs, sessRuns...)
	}
	if !dryRun && len(sessions) > 0 {
		_, _ = a.st.PruneOrphanedEnvironments()
	}
	return sessions, runs, nil
}
